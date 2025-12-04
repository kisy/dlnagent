# DLNA Renderer Agent

This is a simple DLNA renderer agent that allows casting m3u8 streams to DLNA renderers.

## Features

- **Periodic Discovery**: Automatically discovers DLNA renderers on the local network and synchronizes the cache (adds new, removes lost).
- **HTTP API**:
  - `GET /api/devices`: List discovered devices.
  - `POST /api/device/default`: Set a default device for casting.
  - `POST /api/cast`: Cast a media URL to a specific device or the default device. Supports sending a title.
- **Userscript**: Includes a userscript (`m3u8_caster.user.js`) to detect m3u8 videos on web pages and cast them with one click (including page title).
- **Standard Library**: Built using only Go standard library (no external frameworks).

## Usage

### 1. Build and Start the Service

Build the binary:

```bash
./build.sh
```

Run the service (example for Linux AMD64):

```bash
./dlnagent-linux-amd64 -h :8072 -i eth0 -s 10 -p "Living Room"
```

The service will start on port 8072 (default).

- `-h`: HTTP server address (default `:8072`)
- `-i`: Network interface to bind to (e.g., `eth0`)
- `-s`: SSDP search interval in seconds (default `10`)
- `-p`: Default player pattern (matches USN or FriendlyName). Used if no device is specified and no default is set.

### 2. Userscript

1. Install a userscript manager (like Tampermonkey).
2. Install `m3u8_caster.user.js`.
3. Visit a page with an m3u8 video.
4. Click the "Cast to DLNA" button that appears.

### 3. List Devices

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

### 4. Set Default Device

```bash
curl -X POST -d '{"usn": "uuid:..."}' localhost:8072/api/device/default
```

### 5. Cast Media

Cast to default device with title:

```bash
curl -X POST -d '{"url": "http://example.com/video.m3u8", "title": "My Video"}' localhost:8072/api/cast
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
