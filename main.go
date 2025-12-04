package main

import (
	"dlna/api"
	"dlna/dlna"
	"flag"
	"log"
	"net/http"
	"time"
)

func main() {
	addr := flag.String("h", ":8072", "HTTP server address")
	iface := flag.String("i", "eth0", "Network interface to bind to (e.g., eth0)")
	seconds := flag.Int("s", 10, "SSDP search interval in seconds")
	player := flag.String("p", "UnPlay", "Default player pattern (USN or FriendlyName match)")
	flag.Parse()

	discovery := dlna.NewDiscoveryService(*iface, time.Duration(*seconds)*time.Second)
	discovery.Start()

	handler := api.NewHandler(discovery, *player)

	http.HandleFunc("/api/devices", handler.ListDevicesHandler)
	http.HandleFunc("/api/device/default", handler.SetDefaultDeviceHandler)
	http.HandleFunc("/api/cast", handler.CastHandler)

	log.Printf("Starting DLNA service on %s", *addr)
	if err := http.ListenAndServe(*addr, nil); err != nil {
		log.Fatal(err)
	}
}
