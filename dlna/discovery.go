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
	ssdpMulticastAddrV4 = "239.255.255.250:1900"
	ssdpMulticastAddrV6 = "[ff02::c]:1900"
	ssdpSearchMsg       = "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: %s\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 1\r\n" +
		"ST: ssdp:all\r\n" +
		"\r\n"
)

type DiscoveryService struct {
	devices  map[string]*Device
	mu       sync.RWMutex
	bindIP   string
	interval time.Duration
}

func NewDiscoveryService(bindIP string, interval time.Duration) *DiscoveryService {
	return &DiscoveryService{
		devices:  make(map[string]*Device),
		bindIP:   bindIP,
		interval: interval,
	}
}

func (s *DiscoveryService) Start() {
	go s.listenMulticast()
	go s.searchLoop()
	go s.cleanupLoop()
}

func (s *DiscoveryService) searchLoop() {
	// Send immediately
	s.sendSearch()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for range ticker.C {
		s.sendSearch()
	}
}

func (s *DiscoveryService) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for usn, dev := range s.devices {
			if now.Sub(dev.LastSeen) > 5*time.Minute {
				delete(s.devices, usn)
				log.Printf("Device removed (timeout): %s", dev.FriendlyName)
			}
		}
		s.mu.Unlock()
	}
}

func (s *DiscoveryService) sendSearch() {
	ips, err := s.getBindIPs()
	if err != nil {
		log.Printf("Error getting bind IPs: %v", err)
		return
	}

	for _, ip := range ips {
		var addrStr string
		var network string

		if ip.To4() != nil {
			addrStr = ssdpMulticastAddrV4
			network = "udp4"
		} else {
			addrStr = ssdpMulticastAddrV6
			network = "udp6"
		}

		addr, err := net.ResolveUDPAddr(network, addrStr)
		if err != nil {
			log.Printf("Error resolving UDP address %s: %v", addrStr, err)
			continue
		}

		conn, err := net.ListenUDP(network, &net.UDPAddr{IP: ip, Port: 0})
		if err != nil {
			continue
		}

		// Format message with correct HOST
		msg := fmt.Sprintf(ssdpSearchMsg, addrStr)

		if _, err := conn.WriteTo([]byte(msg), addr); err != nil {
			log.Printf("Error sending M-SEARCH from %s: %v", ip, err)
		}
		conn.Close()
	}
}

func (s *DiscoveryService) listenMulticast() {
	// Determine which versions to listen on
	listenV4 := true
	listenV6 := true

	if s.bindIP != "0.0.0.0" && s.bindIP != "" {
		ip := net.ParseIP(s.bindIP)
		if ip != nil {
			if ip.To4() != nil {
				listenV6 = false
			} else {
				listenV4 = false
			}
		}
	}

	if listenV4 {
		go s.listenMulticastProto("udp4", ssdpMulticastAddrV4)
	}
	if listenV6 {
		go s.listenMulticastProto("udp6", ssdpMulticastAddrV6)
	}
}

func (s *DiscoveryService) listenMulticastProto(network, addrStr string) {
	addr, err := net.ResolveUDPAddr(network, addrStr)
	if err != nil {
		log.Printf("Error resolving multicast address %s: %v", addrStr, err)
		return
	}

	iface, err := s.getInterface()
	if err != nil {
		// Only log error if we expected to find an interface but failed.
		// If we are in "all interfaces" mode, getInterface returns nil which is fine for ListenMulticastUDP on some systems,
		// BUT ListenMulticastUDP usually requires an interface.
		// Actually, if s.bindIP is 0.0.0.0, getInterface returns nil.
		// net.ListenMulticastUDP allows iface to be nil to listen on default interface,
		// but for robust discovery we might want to listen on all.
		// However, standard ListenMulticastUDP with nil interface usually works for receiving.
		// Let's proceed.
	}

	conn, err := net.ListenMulticastUDP(network, iface, addr)
	if err != nil {
		log.Printf("Error listening multicast %s: %v", network, err)
		return
	}
	defer conn.Close()

	conn.SetReadBuffer(4096)
	buf := make([]byte, 4096)

	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			log.Printf("Error reading packet: %v", err)
			continue
		}
		s.processPacket(buf[:n])
	}
}

func (s *DiscoveryService) processPacket(data []byte) {
	// Try parsing as Request (NOTIFY)
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(data)))
	if err == nil {
		s.handleHeaders(req.Header)
		return
	}

	// Try parsing as Response (HTTP/1.1 200 OK)
	resp, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(data)), nil)
	if err == nil {
		s.handleHeaders(resp.Header)
		return
	}
}

func (s *DiscoveryService) handleHeaders(header http.Header) {
	usn := header.Get("USN")
	location := header.Get("Location")
	server := header.Get("Server")

	if usn == "" || location == "" {
		return
	}

	uuid := strings.Split(usn, "::")[0]

	s.mu.RLock()
	_, exists := s.devices[uuid]
	s.mu.RUnlock()

	if exists {
		s.mu.Lock()
		if d, ok := s.devices[uuid]; ok {
			d.LastSeen = time.Now()
		}
		s.mu.Unlock()
		return
	}

	// New device, fetch description
	go s.fetchDescription(uuid, location, server)
}

func (s *DiscoveryService) fetchDescription(uuid, location, server string) {
	resp, err := http.Get(location)
	if err != nil {
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
		return
	}

	controlURL := ""
	for _, svc := range desc.Device.ServiceList.Service {
		if strings.Contains(svc.ServiceType, "AVTransport") {
			controlURL = svc.ControlURL
			break
		}
	}

	if controlURL == "" {
		return
	}

	// Normalize ControlURL
	if !strings.HasPrefix(controlURL, "http") {
		baseURL := location
		if lastSlash := strings.LastIndex(location, "/"); lastSlash != -1 {
			baseURL = location[:lastSlash]
		}
		if strings.HasPrefix(controlURL, "/") {
			u, _ := http.NewRequest("GET", location, nil)
			controlURL = fmt.Sprintf("%s://%s%s", u.URL.Scheme, u.URL.Host, controlURL)
		} else {
			controlURL = fmt.Sprintf("%s/%s", baseURL, controlURL)
		}
	}

	dev := &Device{
		USN:          uuid,
		Location:     location,
		FriendlyName: desc.Device.FriendlyName,
		Server:       server,
		LastSeen:     time.Now(),
		ControlURL:   controlURL,
	}

	s.mu.Lock()
	if _, exists := s.devices[uuid]; !exists {
		s.devices[uuid] = dev
		log.Printf("Device added: %s (%s)", dev.FriendlyName, dev.Location)
	}
	s.mu.Unlock()
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

// Helpers

func (s *DiscoveryService) getBindIPs() ([]net.IP, error) {
	if s.bindIP != "0.0.0.0" && s.bindIP != "" {
		ip := net.ParseIP(s.bindIP)
		if ip == nil {
			return nil, fmt.Errorf("invalid bind IP: %s", s.bindIP)
		}
		return []net.IP{ip}, nil
	}

	var ips []net.IP
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		if (iface.Flags&net.FlagUp) == 0 || (iface.Flags&net.FlagMulticast) == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipNet, ok := a.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
				// Collect both IPv4 and IPv6
				ips = append(ips, ipNet.IP)
			}
		}
	}
	return ips, nil
}

func (s *DiscoveryService) getInterface() (*net.Interface, error) {
	if s.bindIP == "0.0.0.0" || s.bindIP == "" {
		return nil, nil // Listen on all interfaces
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			if ipNet, ok := a.(*net.IPNet); ok {
				if ipNet.IP.String() == s.bindIP {
					return &iface, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("interface not found for IP %s", s.bindIP)
}
