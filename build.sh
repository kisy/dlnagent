#!/bin/bash

# Build for Linux x86-64 (amd64)
echo "Building for Linux x86-64..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dlna-proxy-linux-amd64 main.go

# Build for Linux aarch64 (arm64)
echo "Building for Linux aarch64..."
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dlna-proxy-linux-arm64 main.go

echo "Build complete! Binaries are in the current directory."
