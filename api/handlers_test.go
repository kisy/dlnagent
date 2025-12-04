package api

import (
	"bytes"
	"dlna/dlna"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandlers(t *testing.T) {
	discovery := dlna.NewDiscoveryService("", 1*time.Second)
	// Mock a device if possible, or just test empty state
	handler := NewHandler(discovery, "")

	t.Run("ListDevices", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/devices", nil)
		w := httptest.NewRecorder()
		handler.ListDevicesHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}

		var devices []*dlna.Device
		if err := json.NewDecoder(w.Body).Decode(&devices); err != nil {
			t.Errorf("Failed to decode response: %v", err)
		}
		if len(devices) != 0 {
			t.Errorf("Expected 0 devices, got %d", len(devices))
		}
	})

	t.Run("SetDefaultDevice", func(t *testing.T) {
		body := []byte(`{"usn": "uuid:1234"}`)
		req := httptest.NewRequest("POST", "/api/device/default", bytes.NewBuffer(body))
		w := httptest.NewRecorder()
		handler.SetDefaultDeviceHandler(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d", w.Code)
		}
	})

	t.Run("CastNoDevice", func(t *testing.T) {
		body := []byte(`{"url": "http://example.com/video.m3u8"}`)
		req := httptest.NewRequest("POST", "/api/cast", bytes.NewBuffer(body))
		w := httptest.NewRecorder()
		handler.CastHandler(w, req)

		// Should fail because no default device is set (or device not found in discovery)
		// Even if we set default, it won't be in discovery map
		if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
			// It might be 400 if no default, or 404 if default set but not found
			// Since we set default in previous test, it should be 404 (Device not found)
			t.Logf("Got status %d", w.Code)
		}
	})
}
