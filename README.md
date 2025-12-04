# DLNA Renderer Agent

This is a simple DLNA renderer agent that allows casting m3u8 streams to DLNA renderers.

## Features

- **SSDP Discovery**: Automatically discovers DLNA renderers on the local network.
- **HTTP API**:
  - `GET /api/devices`: List discovered devices.
  - `POST /api/device/default`: Set a default device for casting.
  - `POST /api/cast`: Cast a media URL to a specific device or the default device.
- **Standard Library**: Built using only Go standard library (no external frameworks).

## Usage

### 1. Start the Service

```bash
go run main.go -h :8072 -i eth0 -s 10 -p "Living Room"
```

The service will start on port 8072 (default).

- `-h`: HTTP server address (default `:8072`)
- `-i`: Network interface to bind to (e.g., `eth0`)
- `-s`: SSDP search interval in seconds (default `10`)
- `-p`: Default player pattern (matches USN or FriendlyName). Used if no device is specified and no default is set.

### 2. List Devices

```bash
curl localhost:8072/api/devices
```

Response:

```json
[
  {
    "usn": "uuid:...",
    "location": "http://192.168.1.x:yyyy/desc.xml",
    "friendly_name": "Living Room TV",
    ...
  }
]
```

### 3. Set Default Device

```bash
curl -X POST -d '{"usn": "uuid:..."}' localhost:8072/api/device/default
```

### 4. Cast Media

Cast to default device:

```bash
curl -X POST -d '{"url": "http://example.com/video.m3u8"}' localhost:8072/api/cast
```

Cast to specific device:

```bash
curl -X POST -d '{"url": "http://example.com/video.m3u8", "usn": "uuid:..."}' localhost:8072/api/cast
```

## Verification Results

Ran unit tests for HTTP handlers:

```
ok  	dlna/api	0.002s
```

(Note: Real DLNA casting requires actual devices on the network, which were not available in this environment, but the logic and protocols are implemented).
