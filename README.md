<div align="center">

# 📻 YggRadio

**Decentralized Radio Platform on Yggdrasil Mesh Network**

[![Version](https://img.shields.io/badge/version-1.0.0-blue.svg)](https://github.com/JB-SelfCompany/yggradio/releases)
[![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg)](https://go.dev/)
[![License](https://img.shields.io/badge/license-GPLv3-green.svg)](LICENSE)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](https://github.com/JB-SelfCompany/yggradio/pulls)

*Self-hosted, privacy-first radio streaming over the Yggdrasil encrypted mesh network*

[Features](#-features) •
[Installation](#-installation) •
[Quick Start](#-quick-start)

</div>

---

## ✨ Features

- 🔒 **End-to-End Encrypted** - All traffic automatically encrypted via Yggdrasil (TLS 1.3)
- 🚫 **No Central Servers** - Fully decentralized P2P architecture
- 🎵 **Multi-Format Streaming** - MP3, Ogg Vorbis, Opus, AAC, FLAC support
- 🔐 **Dual Authentication** - Ed25519 signature-based OR magic link authentication
- 🔑 **Privacy-First** - No usernames, emails, or personal data required
- 🌐 **Federation Support** - Optional hub-and-spoke federation for discovery
- 📦 **Single Binary** - No external dependencies except Yggdrasil daemon
- 🎨 **Modern Web UI** - React-based, responsive interface
- 🔥 **Real-time Listeners** - Live listener count and station statistics
- 🛡️ **Security Hardened** - Rate limiting, CSRF protection, XSS prevention
- ⚡ **Low Latency** - Optimized buffers for near-real-time streaming

---

## 📋 Table of Contents

- [Requirements](#-requirements)
- [Installation](#-installation)
  - [From Binary](#from-binary)
  - [From Source](#from-source)
  - [Using Systemd](#using-systemd)
- [Quick Start](#-quick-start)
- [Authentication](#-authentication)
  - [Ed25519 Key Pairs](#ed25519-key-pairs)
  - [Magic Link](#magic-link)
- [Configuration](#-configuration)
- [Streaming](#-streaming)
  - [Broadcasting](#broadcasting)
  - [Listening](#listening)
- [Federation](#-federation)
- [Architecture](#-architecture)
- [Development](#-development)
- [License](#-license)
- [Support](#-support)

---

## 🔧 Requirements

- **Yggdrasil daemon** running on the same machine ([Install Guide](https://yggdrasil-network.github.io/installation.html))
- **Go 1.21+** (for building from source)
- **Node.js** (for frontend development only)

---

## 📦 Installation

### From Binary

Download the latest release for your platform:

```bash
# Linux/macOS
wget https://github.com/JB-SelfCompany/yggradio/releases/download/v1.0.0/yggradio-linux-amd64.tar.gz
tar -xzf yggradio-linux-amd64.tar.gz
sudo mv yggradio /usr/local/bin/

# Windows
# Download from releases page and add to PATH
```

### From Source

```bash
# Clone repository
git clone https://github.com/JB-SelfCompany/yggradio.git
cd yggradio

# Build (includes frontend)
bash build.sh  # Linux/macOS

# Binary will be in bin/yggradio
```

### Using Systemd

For production deployments on Linux, use systemd to run YggRadio as a service:

#### YggRadio Service

```bash
# Create system user
sudo useradd -r -s /bin/false yggradio

# Copy the binary file and make it executable
sudo cp bin/yggradio-linux-amd64 /usr/local/bin/yggradio-linux
sudo chmod +x /usr/local/bin/yggradio-linux

# Create configuration directory
sudo mkdir -p /home/yggradio/.yggradio
sudo cp config.example.yaml /home/yggradio/.yggradio/config.yaml
sudo chown -R yggradio:yggradio /home/yggradio
# Edit if needed: sudo nano /home/yggradio/.yggradio/config.yaml

# Copy systemd service file
sudo cp systemd/yggradio.service /etc/systemd/system/

# Reload systemd
sudo systemctl daemon-reload

# Enable service (start on boot)
sudo systemctl enable yggradio

# Start service
sudo systemctl start yggradio

# Check status
sudo systemctl status yggradio

# View logs
sudo journalctl -u yggradio -f
```

#### Federation Server Service

```bash
# Create system user
sudo useradd -r -s /bin/false yggradio-federation

# Copy the binary file and make it executable
sudo cp bin/yggradio-federation-server-linux-amd64 /usr/local/bin/yggradio-federation-server
sudo chmod +x /usr/local/bin/yggradio-federation-server

# Create configuration directory
sudo mkdir -p /home/yggradio-federation/.yggradio-federation
sudo cp config-federation.example.yaml /home/yggradio-federation/.yggradio-federation/config.yaml
sudo chown -R yggradio-federation:yggradio-federation /home/yggradio-federation
# Edit if needed: sudo nano /home/yggradio-federation/.yggradio-federation/config.yaml

# Copy systemd service file
sudo cp systemd/yggradio-federation-server.service /etc/systemd/system/

# Reload systemd
sudo systemctl daemon-reload

# Enable service (start on boot)
sudo systemctl enable yggradio-federation-server

# Start service
sudo systemctl start yggradio-federation-server

# Check status
sudo systemctl status yggradio-federation-server

# View logs
sudo journalctl -u yggradio-federation-server -f
```

**Note:** Both services run under dedicated system users with security hardening enabled (NoNewPrivileges, ProtectSystem, PrivateTmp). Configuration files are stored in the user's home directory (`~/.yggradio/` or `~/.yggradio-federation/`).

---

## 🚀 Quick Start

1. **Install and start Yggdrasil daemon:**
   ```bash
   # See: https://yggdrasil-network.github.io/installation.html
   sudo systemctl start yggdrasil
   ```

2. **Start YggRadio:**
   ```bash
   yggradio
   ```

3. **Access the web interface:**
   - YggRadio will display its URL on startup (e.g., `http://[200:xxxx:xxxx:xxxx::1]:8080`)
   - Open this URL in your browser from any Yggdrasil-connected device

4. **Create your first station:**
   - Click "Create Station" in the web UI
   - Start broadcasting with ffmpeg, OBS, or BUTT

---

## 🔐 Authentication

YggRadio supports **two authentication methods** - choose the one that fits your needs:

### Ed25519 Key Pairs

**Privacy-focused cryptographic authentication with no passwords**

#### Browser-Generated Keys (Quick & Easy)

1. Click **"Login"** → **"Generate New Keys"**
2. **Save your keys securely** (download the JSON file)
3. Keys are stored in `sessionStorage` (cleared when browser closes)

#### Manually-Generated Keys (Most Secure)

For maximum security, generate keys outside the browser:

**Python (PyNaCl):**
```bash
python3 -c "import nacl.signing, base64; \
key = nacl.signing.SigningKey.generate(); \
print('Private:', base64.b64encode(bytes(key)).decode()); \
print('Public:', base64.b64encode(bytes(key.verify_key)).decode())"
```

**Node.js (tweetnacl):**
```bash
node -e "const nacl = require('tweetnacl'); \
const key = nacl.sign.keyPair(); \
console.log('Private:', Buffer.from(key.secretKey).toString('base64')); \
console.log('Public:', Buffer.from(key.publicKey).toString('base64'));"
```

**OpenSSL:**
```bash
openssl genpkey -algorithm ed25519 -out key.pem
openssl pkey -in key.pem -pubout -out pubkey.pem
# Note: Convert PEM to base64 manually
```

**Import Keys:**
1. Click **"Login"** → **"Import Keys"**
2. Paste your public and private keys (base64 format)
3. Or upload the JSON file

**Security:**
- ✅ Private keys never leave your device
- ✅ No passwords to remember or compromise
- ✅ Cryptographic signatures for every request
- ✅ Automatic replay protection (5-minute window)

---

### Magic Link

**Simple bookmark-based authentication for easy access**

#### Generate a Magic Link

1. Click **"Login"** → **"Magic Link"**
2. Click **"Generate Magic Link"**
3. Wait for Proof-of-Work calculation (~2-4 seconds)
4. **Save the link securely** (bookmark or download)

#### Use Your Magic Link

1. Visit the saved link in any browser
2. A session cookie is created automatically (1 week expiration)
3. Visit the link again anytime to refresh your session

**Security Notes:**
- ⚠️ **Anyone with the link can access your account** - store it securely
- 🔒 Magic link never expires, but cookies do (1 week)
- 🔐 Tokens and cookies stored as SHA256 hashes (192-bit and 256-bit entropy)
- 🛡️ Constant-time comparison prevents timing attacks
- 📝 Recommended: Store in password manager or bookmark securely

**When to Use:**
- ✅ Quick access from multiple devices
- ✅ Don't want to manage cryptographic keys
- ✅ Prefer bookmark-style authentication
- ❌ Highest security requirements (use Ed25519 instead)

---

## ⚙️ Configuration

Configuration file location: `~/.yggradio/config.yaml`

On first run, YggRadio automatically creates a default configuration file. You can also create it manually:

```bash
# Copy example configuration
cp config.example.yaml ~/.yggradio/config.yaml

# Edit configuration
nano ~/.yggradio/config.yaml
```

**Key settings:**

```yaml
server:
  port: 8080
  bind: ""  # Auto-detect Yggdrasil IPv6 address
  instance_name: "My YggRadio"

streaming:
  max_listeners_per_station: 100
  max_source_clients: 10
  buffer_size: 32768
  server_secret: ""  # Auto-generated on first run

security:
  magic_link_enabled: true  # Enable magic link authentication
  magic_link_token_length: 24  # 24 bytes = 48 hex chars (192 bits)
  magic_link_cookie_ttl: 604800  # 1 week in seconds
  magic_link_require_pow: true  # Require PoW for spam protection
  magic_link_pow_difficulty: 16  # Same as station creation

federation:
  enabled: false  # Set to true to join federation
  server_address: "301:be28:cf55:3c9::10"
  server_port: 9000
```

**Full configuration example:** See [config.example.yaml](config.example.yaml)

### Federation Server Configuration

For running a federation server:

```bash
# Copy example configuration
cp config-federation.example.yaml ~/.yggradio-federation/config.yaml

# Edit configuration
nano ~/.yggradio-federation/config.yaml
```

**Full federation configuration example:** See [config-federation.example.yaml](config-federation.example.yaml)

---

## 🎙️ Streaming

### Broadcasting

Use any streaming source client that supports HTTP streaming:

**ffmpeg example:**
```bash
ffmpeg -re -i music.mp3 -codec:a libmp3lame -b:a 128k \
  -f mp3 http://[YOUR_YGGDRASIL_IP]:8080/your-mountpoint \
  -user username -password your-source-password
```

**OBS Studio:**
1. Settings → Stream
2. Service: Custom
3. Server: `http://[YOUR_YGGDRASIL_IP]:8080/your-mountpoint`
4. Stream Key: Use HTTP Basic Auth

**BUTT (Broadcast Using This Tool):**
1. Settings → Server → Icecast
2. Address: Your Yggdrasil IPv6
3. Port: 8080
4. Mountpoint: /your-mountpoint

### Listening

Open the station URL in your web browser:
```
http://[BROADCASTER_YGGDRASIL_IP]:8080/
```

The web player will automatically load and stream audio.

---

## 🌐 Federation

Federation allows YggRadio instances to discover each other through a central hub.

### Joining a Federation

1. Edit `~/.yggradio/config.yaml`:
   ```yaml
   federation:
     enabled: true
     server_address: "301:be28:cf55:3c9::10"  # Federation server address
     server_port: 9000
   ```

2. Restart YggRadio:
   ```bash
   sudo systemctl restart yggradio
   ```

### Running a Federation Server

```bash
# Install federation server
sudo cp yggradio-federation-server /usr/local/bin/

# Start with systemd
sudo cp systemd/yggradio-federation-server.service /etc/systemd/system/
sudo systemctl enable --now yggradio-federation-server
```

Configuration: `~/.yggradio-federation/config.yaml`

---

## 🏗️ Architecture

### Standalone Mode (Default)
Each YggRadio instance works independently without requiring any external services:

```
┌─────────────────────────────────────────────────────────┐
│                    Yggdrasil Network                     │
│              (Encrypted Mesh Network)                    │
└─────────────────────────────────────────────────────────┘
                          │
    ┌─────────────────────┼─────────────────────┐
    │                     │                     │
┌───▼────┐          ┌─────▼─────┐        ┌─────▼─────┐
│ Client │          │ YggRadio  │        │ YggRadio  │
│Browser │◄────────►│  Node 1   │        │  Node 2   │
└────────┘          │(Standalone)│        │(Standalone)│
                    └─────┬─────┘        └─────┬─────┘
                          │                    │
                          ▼                    ▼
                     ┌─────────┐          ┌─────────┐
                     │ SQLite  │          │ SQLite  │
                     │   DB    │          │   DB    │
                     └─────────┘          └─────────┘
```

### Federation Mode (Optional)
Enable federation for automatic station discovery across the network:

```
                 ┌──────────────────────────┐
                 │   Federation Server      │
                 │  (Optional Discovery)    │
                 │  - Station Registry      │
                 │  - Node Heartbeats       │
                 └────────┬─────────────────┘
                          │
         ┌────────────────┼────────────────┐
         │                │                │
    ┌────▼─────┐     ┌────▼─────┐    ┌────▼─────┐
    │YggRadio 1│◄───►│YggRadio 2│◄───►│YggRadio 3│
    │(Federated)│     │(Federated)│    │(Federated)│
    └────┬─────┘     └────┬─────┘    └────┬─────┘
         │                │                │
    ┌────▼─────┐     ┌────▼─────┐    ┌────▼─────┐
    │ SQLite   │     │ SQLite   │    │ SQLite   │
    │   DB     │     │   DB     │    │   DB     │
    └──────────┘     └──────────┘    └──────────┘

    Node 1 ◄──────► Node 2 ◄──────► Node 3
    (Direct station streaming between nodes)
```

**Deployment Options:**
- **Standalone**: Each instance operates independently (default)
- **Federated**: Nodes register with federation server for network-wide discovery
- **Hybrid**: Federation server on same host using `localhost` for zero latency

**Key Components:**

- **Yggdrasil Integration**: Auto-detects IPv6 address, all traffic encrypted
- **HTTP Streaming Server**: Accepts source clients and serves listeners
- **Web UI**: React + TypeScript frontend (embedded in binary)
- **Database**: SQLite for stations, users, metadata (privacy-first minimal schema)
- **Dual Authentication**: Ed25519 signatures OR magic link + session cookies
- **Security Layer**: CSRF protection, rate limiting, XSS prevention, audit logging
- **Federation Client**: Optional discovery via federation hub (disabled by default)

**Privacy Features:**
- ✅ No usernames, emails, or personal information required
- ✅ Ed25519 private keys never leave your device
- ✅ Magic link tokens and cookies stored as SHA256 hashes only
- ✅ Minimalist database schema (id, pubkey/null, timestamps only)
- ✅ Session-only key storage (sessionStorage, not localStorage)
- ✅ IP addresses excluded from audit logs (privacy-focused logging)

---

## 🛠️ Development

### Project Structure

```
yggradio/
├── cmd/
│   ├── yggradio/                      # Main application
│   └── yggradio-federation-server/    # Federation server
├── internal/
│   ├── api/
│   │   ├── handlers/                  # HTTP request handlers
│   │   └── middleware/                # Auth, rate limiting, CSRF
│   ├── config/                        # Configuration management
│   ├── database/
│   │   ├── models/                    # Database repositories
│   │   └── schema.sql                 # Database schema
│   ├── federation_client/             # Federation client
│   ├── federation_server/             # Federation server
│   ├── moderation/                    # PoW & content filtering
│   ├── security/                      # Auth, CSRF, validation, sanitization
│   ├── streaming/                     # HTTP streaming server
│   ├── testutil/                      # Testing utilities
│   ├── utils/                         # Helper functions
│   └── web/
│       └── dist/                      # Embedded frontend (built)
├── web/                               # React frontend source
│   ├── src/
│   │   ├── components/                # React components
│   │   ├── lib/                       # API client, utilities
│   │   ├── pages/                     # Page components
│   │   └── stores/                    # Zustand state stores
│   └── dist/                          # Frontend build output
├── systemd/                           # Systemd service files
├── bin/                               # Compiled binaries (gitignored)
└── dist/                              # Release archives (gitignored)
```

### Building

```bash
# Full build (frontend + backend for all platforms)
bash build.sh

# Backend only
go build -o bin/yggradio ./cmd/yggradio

# Frontend only
cd web && npm run build

# Frontend dev server (with hot reload)
cd web && npm run dev

# Backend with auto-reload (requires air)
air
```

### Testing

```bash
# All tests with race detector
go test -v -race ./...

# Unit tests only (fast)
go test -v -race -short ./...

# Security tests
go test -v -race ./internal/security/...

# Frontend tests
cd web && npm test

# Coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Code Quality

```bash
# Format
go fmt ./...

# Lint
go vet ./...
cd web && npm run lint

```

---

## 📄 License

This project is licensed under the GNU General Public License v3.0 - see the [LICENSE](LICENSE) file for details.

YggRadio is free software: you can redistribute it and/or modify it under the terms of the GNU General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

---

## 💬 Support

- **Issues**: [GitHub Issues](https://github.com/JB-SelfCompany/yggradio/issues)
- **Discussions**: [GitHub Discussions](https://github.com/JB-SelfCompany/yggradio/discussions)
- **Yggdrasil Network**: [yggdrasil-network.github.io](https://yggdrasil-network.github.io/)

---

## 🙏 Acknowledgments

- [Yggdrasil Network](https://yggdrasil-network.github.io/) - Encrypted mesh network
- [modernc.org/sqlite](https://modernc.org/sqlite) - Pure Go SQLite

---

<div align="center">

**Made with ❤️ for the decentralized web**

⭐ Star us on GitHub — it helps!

</div>