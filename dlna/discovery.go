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
	devices       map[string]*Device
	lastLogged    map[string]time.Time
	mu            sync.RWMutex
	ifaceName     string
	interval      time.Duration
	scanDuration  time.Duration
	candidateChan chan *candidate
	resultChan    chan *Device
}

type candidate struct {
	usn      string
	location string
}

func NewDiscoveryService(ifaceName string, interval time.Duration) *DiscoveryService {
	return &DiscoveryService{
		devices:       make(map[string]*Device),
		lastLogged:    make(map[string]time.Time),
		ifaceName:     ifaceName,
		interval:      interval,
		scanDuration:  3 * time.Second,
		candidateChan: make(chan *candidate, 10),
		resultChan:    make(chan *Device, 10),
	}
}

func (s *DiscoveryService) Start() {
	go s.listenMulticast()
	go s.runLoop()
}

func (s *DiscoveryService) runLoop() {
	for {
		// 1. Start Round
		roundFound := make(map[string]bool)
		var roundNew []*Device

		// 2. Trigger Search
		go s.sendSearch()

		// 3. Collect Phase
		timeout := time.After(s.scanDuration)
	collectLoop:
		for {
			select {
			case cand := <-s.candidateChan:
				if roundFound[cand.usn] {
					continue
				}
				roundFound[cand.usn] = true

				s.mu.RLock()
				_, exists := s.devices[cand.usn]
				s.mu.RUnlock()

				if exists {
					// Cache has skip
					continue
				}

				// Not exist add
				go s.fetchDescription(cand.usn, cand.location)

			case dev := <-s.resultChan:
				if dev != nil {
					roundNew = append(roundNew, dev)
					roundFound[dev.USN] = true
				}

			case <-timeout:
				break collectLoop
			}
		}

		// 4. Sync Phase
		s.mu.Lock()
		// Delete missing
		for usn := range s.devices {
			if !roundFound[usn] {
				delete(s.devices, usn)
				log.Printf("Device removed: %s", usn)
			}
		}
		// Add new
		for _, dev := range roundNew {
			s.devices[dev.USN] = dev
			log.Printf("Device added: %s (%s)", dev.FriendlyName, dev.Location)
		}
		s.mu.Unlock()

		// 5. Sleep
		time.Sleep(s.interval)
	}
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

	s.handleHeaders(header)
}

func (s *DiscoveryService) handleHeaders(header http.Header) {
	usn := header.Get("USN")
	location := header.Get("Location")

	if usn == "" || location == "" {
		return
	}

	// Extract UUID from USN
	uuid := strings.Split(usn, "::")[0]

	// Send to candidate channel
	select {
	case s.candidateChan <- &candidate{usn: uuid, location: location}:
	default:
		// Drop if channel full to avoid blocking listener
	}
}

func (s *DiscoveryService) fetchDescription(uuid, location string) {
	// log.Printf("Fetching description from %s for UUID %s", location, uuid)
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

	if err := xml.NewDecoder(resp.Body).Decode(&desc); err != nil {
		log.Printf("Error decoding description XML from %s: %v", location, err)
		return
	}

	controlURL := ""
	for _, svc := range desc.Device.ServiceList.Service {
		if strings.Contains(svc.ServiceType, "AVTransport") {
			controlURL = svc.ControlURL
		}
	}

	if controlURL == "" {
		log.Printf("No AVTransport service found for %s", location)
		return
	}

	// Handle relative ControlURL
	if !strings.HasPrefix(controlURL, "http") {
		baseURL := location
		if lastSlash := strings.LastIndex(location, "/"); lastSlash != -1 {
			baseURL = location[:lastSlash]
		}
		if strings.HasPrefix(controlURL, "/") {
			if u, err := http.NewRequest("GET", location, nil); err == nil {
				controlURL = fmt.Sprintf("%s://%s%s", u.URL.Scheme, u.URL.Host, controlURL)
			}
		} else {
			controlURL = fmt.Sprintf("%s/%s", baseURL, controlURL)
		}
	}

	dev := &Device{
		USN:          uuid,
		Location:     location,
		FriendlyName: desc.Device.FriendlyName,
		LastSeen:     time.Now(),
		ControlURL:   controlURL,
	}

	select {
	case s.resultChan <- dev:
	case <-time.After(1 * time.Second):
		log.Printf("Timeout sending device result for %s", uuid)
	}
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
