package api

import (
	"dlna/dlna"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

type Handler struct {
	discovery      *dlna.DiscoveryService
	defaultID      string
	defaultPattern string
	mu             sync.RWMutex
}

func NewHandler(d *dlna.DiscoveryService, pattern string) *Handler {
	return &Handler{
		discovery:      d,
		defaultPattern: pattern,
	}
}

func (h *Handler) ListDevicesHandler(w http.ResponseWriter, r *http.Request) {
	devices := h.discovery.GetDevices()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(devices)
}

func (h *Handler) SetDefaultDeviceHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		USN string `json:"usn"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	h.defaultID = req.USN
	h.mu.Unlock()

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Default device set to %s", req.USN)
}

func (h *Handler) CastHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
		USN string `json:"usn"` // Optional
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	targetUSN := req.USN

	// 1. Try explicit USN
	// 2. Try manually set defaultID
	if targetUSN == "" {
		h.mu.RLock()
		targetUSN = h.defaultID
		h.mu.RUnlock()
	}

	// 3. Try pattern match if no default set
	if targetUSN == "" && h.defaultPattern != "" {
		devices := h.discovery.GetDevices()
		for _, d := range devices {
			if strings.Contains(d.USN, h.defaultPattern) || strings.Contains(d.FriendlyName, h.defaultPattern) {
				targetUSN = d.USN
				break
			}
		}
	}

	if targetUSN == "" {
		http.Error(w, "Please specify a device or set a default device first.", http.StatusBadRequest)
		return
	}

	device := h.discovery.GetDevice(targetUSN)
	if device == nil {
		http.Error(w, "Device not found", http.StatusNotFound)
		return
	}

	if err := dlna.Play(device.ControlURL, req.URL); err != nil {
		http.Error(w, fmt.Sprintf("Failed to cast: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Casting to %s", device.FriendlyName)
}
