package dlna

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"fmt"

	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	ssdpMulticastAddr = "239.255.255.250:1900"
	ssdpSearchMsg     = "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 1\r\n" +
		"ST: ssdp:all\r\n" +
		"\r\n"
)

type DiscoveryService struct {
	devices    map[string]*Device
	lastLogged map[string]time.Time
	mu         sync.RWMutex
	ifaceName  string
	interval   time.Duration
}

func NewDiscoveryService(ifaceName string, interval time.Duration) *DiscoveryService {
	return &DiscoveryService{
		devices:    make(map[string]*Device),
		lastLogged: make(map[string]time.Time),
		ifaceName:  ifaceName,
		interval:   interval,
	}
}

func (s *DiscoveryService) Start() {
	go s.listenMulticast()
	go s.sendSearch()
}

// sendSearch sends a single burst of M-SEARCH packets
func (s *DiscoveryService) sendSearch() {
	addr, err := net.ResolveUDPAddr("udp", ssdpMulticastAddr)
	if err != nil {
		log.Printf("Error resolving UDP address: %v", err)
		return
	}

	// Send on all interfaces or specific interface
	var ifaces []net.Interface
	if s.ifaceName != "" {
		iface, err := net.InterfaceByName(s.ifaceName)
		if err != nil {
			log.Printf("Error getting interface %s: %v", s.ifaceName, err)
			return
		}
		ifaces = []net.Interface{*iface}
	} else {
		var err error
		ifaces, err = net.Interfaces()
		if err != nil {
			log.Printf("Error listing interfaces: %v", err)
			return
		}
	}

	for {
		for _, iface := range ifaces {
			if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagMulticast) == 0 {
				continue
			}

			conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4zero, Port: 0, Zone: iface.Name})
			if err != nil {
				// log.Printf("Error listening UDP on %s: %v", iface.Name, err)
				continue
			}

			_, err = conn.WriteTo([]byte(ssdpSearchMsg), addr)
			conn.Close()
			if err != nil {
				log.Printf("Error sending M-SEARCH on %s: %v", iface.Name, err)
			} else {
				// log.Printf("Sent M-SEARCH on %s", iface.Name)
			}
		}
		time.Sleep(s.interval)
	}
}

func (s *DiscoveryService) listenMulticast() {
	addr, err := net.ResolveUDPAddr("udp", ssdpMulticastAddr)
	if err != nil {
		log.Printf("Error resolving multicast address: %v", err)
		return
	}

	var iface *net.Interface
	if s.ifaceName != "" {
		var err error
		iface, err = net.InterfaceByName(s.ifaceName)
		if err != nil {
			log.Printf("Error getting interface %s: %v", s.ifaceName, err)
			return
		}
	}

	conn, err := net.ListenMulticastUDP("udp", iface, addr)
	if err != nil {
		log.Printf("Error listening multicast: %v", err)
		return
	}
	defer conn.Close()

	// Set read buffer size to handle burst of packets
	conn.SetReadBuffer(4096)

	// log.Println("Listening for SSDP packets...")
	buf := make([]byte, 4096)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading from UDP: %v", err)
			continue
		}
		// Log concise packet info
		// log.Printf("Received packet from %v", src)
		s.processPacket(buf[:n], src)
	}
}

func (s *DiscoveryService) processPacket(data []byte, src net.Addr) {
	// Simple parsing of HTTP headers from the packet
	reader := bufio.NewReader(bytes.NewReader(data))
	req, err := http.ReadResponse(reader, nil)

	var header http.Header
	if err != nil {
		// Try reading as Request if Response fails (NOTIFY packets are requests)
		// Must create a NEW reader because the previous one might have been consumed
		reader2 := bufio.NewReader(bytes.NewReader(data))
		req2, err2 := http.ReadRequest(reader2)
		if err2 != nil {
			// log.Printf("Failed to parse packet from %v: %v", src, err2)
			return
		}
		header = req2.Header
	} else {
		header = req.Header
	}

	usn := header.Get("USN")
	if usn != "" {
		// Extract UUID for logging deduplication
		uuid := strings.Split(usn, "::")[0]
		s.mu.Lock()
		last, ok := s.lastLogged[uuid]
		shouldLog := !ok || time.Since(last) > 30*time.Second
		if shouldLog {
			log.Printf("Received packet from %v:\n%s", src, string(data))
			s.lastLogged[uuid] = time.Now()
		}
		s.mu.Unlock()
	}

	s.handleHeaders(header, src)
}

func (s *DiscoveryService) handleHeaders(header http.Header, src net.Addr) {
	usn := header.Get("USN")
	location := header.Get("Location")

	if usn == "" || location == "" {
		return
	}

	// Extract UUID from USN
	// Format is usually uuid:device-UUID::... or just uuid:device-UUID
	uuid := strings.Split(usn, "::")[0]

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.devices[uuid]; !exists {
		log.Printf("Found potential device: %s at %s (from %v)", uuid, location, src)
		// New device, fetch description
		go s.fetchDescription(uuid, location)
	} else {
		// Update last seen
		s.devices[uuid].LastSeen = time.Now()
		// Update location if changed (sometimes devices change ports)
		if s.devices[uuid].Location != location {
			s.devices[uuid].Location = location
			// Optionally re-fetch description if location changed
		}
	}
}

func (s *DiscoveryService) fetchDescription(uuid, location string) {
	log.Printf("Fetching description from %s for UUID %s", location, uuid)
	resp, err := http.Get(location)
	if err != nil {
		log.Printf("Error fetching description from %s: %v", location, err)
		return
	}
	defer resp.Body.Close()

	var desc struct {
		Device struct {
			FriendlyName string `xml:"friendlyName"`
			ServiceList  struct {
				Service []struct {
					ServiceType string `xml:"serviceType"`
					ControlURL  string `xml:"controlURL"`
				} `xml:"service"`
			} `xml:"serviceList"`
		} `xml:"device"`
	}

	// Read body for debugging (optional, but helpful if XML is malformed)
	// bodyBytes, _ := io.ReadAll(resp.Body)
	// log.Printf("XML Content: %s", string(bodyBytes))
	// resp.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if err := xml.NewDecoder(resp.Body).Decode(&desc); err != nil {
		log.Printf("Error decoding description XML from %s: %v", location, err)
		return
	}

	log.Printf("Parsed device: %s", desc.Device.FriendlyName)

	controlURL := ""
	for _, svc := range desc.Device.ServiceList.Service {
		// log.Printf("Found service: %s", svc.ServiceType)
		if strings.Contains(svc.ServiceType, "AVTransport") {
			controlURL = svc.ControlURL
			// Don't break yet, maybe we want to log all services
			// break
		}
	}

	if controlURL == "" {
		log.Printf("No AVTransport service found for %s", location)
		return
	}

	// Handle relative ControlURL
	if !strings.HasPrefix(controlURL, "http") {
		// Simple join, might need more robust URL handling
		baseURL := location
		if lastSlash := strings.LastIndex(location, "/"); lastSlash != -1 {
			baseURL = location[:lastSlash]
		}
		// If controlURL starts with /, use host from location
		if strings.HasPrefix(controlURL, "/") {
			// Extract host
			if u, err := http.NewRequest("GET", location, nil); err == nil {
				controlURL = fmt.Sprintf("%s://%s%s", u.URL.Scheme, u.URL.Host, controlURL)
			}
		} else {
			controlURL = fmt.Sprintf("%s/%s", baseURL, controlURL)
		}
	}

	s.mu.Lock()
	s.devices[uuid] = &Device{
		USN:          uuid, // Store UUID as USN identifier
		Location:     location,
		FriendlyName: desc.Device.FriendlyName,
		LastSeen:     time.Now(),
		ControlURL:   controlURL,
	}
	s.mu.Unlock()
	log.Printf("Discovered device: %s (%s)", desc.Device.FriendlyName, location)
}

func (s *DiscoveryService) GetDevices() []*Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	devices := make([]*Device, 0, len(s.devices))
	for _, d := range s.devices {
		devices = append(devices, d)
	}
	return devices
}

func (s *DiscoveryService) GetDevice(usn string) *Device {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.devices[usn]
}
