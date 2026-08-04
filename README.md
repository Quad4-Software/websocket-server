# Websocket Server

A WebSocket server in Go.

## Features

- WebSocket relay server with multiple operation modes: echo, broadcast, and log
- Rate limiting per connection and global
- Reverse proxy support (X-Forwarded-* headers)
- Privacy-focused: no IP address logging of connecting clients
- Configurable via environment variables or command-line flags

## Installation

```bash
go build -o bin/server ./cmd/server
```

## Usage

### Environment Variables

- `HOST` - Server host (default: `0.0.0.0`)
- `PORT` - Server port (default: `8080`)
- `MODE` - Operation mode: `echo`, `broadcast`, or `log` (default: `echo`)
- `DOMAIN` - Domain name for CORS and WSS enforcement (default: empty)
- `RATE_LIMIT` - Rate limit per second (default: `10`)
- `BURST` - Burst size for rate limiting (default: `20`)
- `CLIENT_LIMIT` - Maximum concurrent clients (default: `1000`)

### Command-Line Flags

All environment variables can be overridden with command-line flags:

- `--host` - Server host
- `--port` - Server port
- `--mode` - Operation mode
- `--domain` - Domain name
- `--rate-limit` - Rate limit per second
- `--burst` - Burst size
- `--client-limit` - Maximum concurrent clients

### Precedence

Configuration precedence: Environment Variables > Command-Line Flags > Defaults

### Examples

```bash
# Using environment variables
export PORT=9000
export MODE=broadcast
export DOMAIN=example.com
./bin/server

# Using command-line flags
./bin/server --port 9000 --mode broadcast --domain example.com

# Development mode (no domain/WSS enforcement)
./bin/server --mode echo --port 8080
```

## Operation Modes

- **echo**: Echoes received packets back to the sender
- **broadcast**: Broadcasts received packets to all connected clients except the sender
- **log**: Logs received packets (first 32 bytes in hex)

## Development

### Prerequisites

- Go 1.25.5 or later

### Running Linters

Requires: revive, gosec

```bash
task lint
```

Individual linters:

```bash
task fmt
task vet
task gosec
task revive
```

### Building

```bash
task build
```

### Running

```bash
task run
```

## Docker

Build and run with Docker:

```bash
docker build -t ws-relay .
docker run -p 8080:8080 -e PORT=8080 -e MODE=echo ws-relay
```

## License

BSD 3-Clause License (c) 2025 Quad4.io
