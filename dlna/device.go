package dlna

import (
	"time"
)

// Device represents a DLNA/UPnP device
type Device struct {
	USN          string    `json:"usn"`
	Location     string    `json:"location"`
	Server       string    `json:"server"`
	FriendlyName string    `json:"friendly_name"`
	LastSeen     time.Time `json:"last_seen"`
	ControlURL   string    `json:"control_url"` // AVTransport Control URL
}
