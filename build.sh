#!/bin/bash

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Version from internal/version/version.go (single source of truth)
VERSION_FILE="internal/version/version.go"
if [ -f "$VERSION_FILE" ]; then
    VERSION=$(grep -oP 'var Version = "\K[^"]+' "$VERSION_FILE" 2>/dev/null || echo "dev")
else
    VERSION="dev"
fi
VERSION_PKG="github.com/JB-SelfCompany/yggradio/internal/version"
BIN_DIR="bin"
WEB_DIR="web"
DIST_DIR="dist"

echo -e "${GREEN}YggRadio Build Script${NC}"
echo -e "${GREEN}Version: ${VERSION}${NC}"
echo ""

# Check dependencies
echo -e "${YELLOW}Checking dependencies...${NC}"
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    exit 1
fi

if ! command -v npm &> /dev/null; then
    echo -e "${RED}Error: npm is not installed${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Dependencies OK${NC}"
echo ""

# Download Go dependencies
echo -e "${YELLOW}Downloading Go dependencies...${NC}"
go mod download
go mod verify
echo -e "${GREEN}✓ Go dependencies downloaded${NC}"
echo ""

# Build frontend
echo -e "${YELLOW}Building frontend...${NC}"
cd "${WEB_DIR}"
npm install
npm run build
cd ..
echo -e "${GREEN}✓ Frontend built${NC}"
echo ""

# Copy frontend dist to internal/web for embedding
echo -e "${YELLOW}Copying frontend for embedding...${NC}"
rm -rf internal/web/dist
cp -r web/dist internal/web/dist
echo -e "${GREEN}✓ Frontend copied to internal/web/dist${NC}"
echo ""

# Create output directories
mkdir -p "${BIN_DIR}"
mkdir -p "${DIST_DIR}"

# Build for all platforms
echo -e "${YELLOW}Building binaries for all platforms...${NC}"
echo ""

# Linux AMD64
echo -e "${YELLOW}Building Linux AMD64...${NC}"
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-X ${VERSION_PKG}.Version=${VERSION} -s -w" -o "${BIN_DIR}/yggradio-linux-amd64" ./cmd/yggradio
echo -e "${GREEN}✓ Linux AMD64 built${NC}"

# Linux ARM64
echo -e "${YELLOW}Building Linux ARM64...${NC}"
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-X ${VERSION_PKG}.Version=${VERSION} -s -w" -o "${BIN_DIR}/yggradio-linux-arm64" ./cmd/yggradio
echo -e "${GREEN}✓ Linux ARM64 built${NC}"

# Windows AMD64
echo -e "${YELLOW}Building Windows AMD64...${NC}"
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-X ${VERSION_PKG}.Version=${VERSION} -s -w" -o "${BIN_DIR}/yggradio-windows-amd64.exe" ./cmd/yggradio
echo -e "${GREEN}✓ Windows AMD64 built${NC}"

# macOS AMD64
echo -e "${YELLOW}Building macOS AMD64...${NC}"
GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-X ${VERSION_PKG}.Version=${VERSION} -s -w" -o "${BIN_DIR}/yggradio-darwin-amd64" ./cmd/yggradio
echo -e "${GREEN}✓ macOS AMD64 built${NC}"

# macOS ARM64 (Apple Silicon)
echo -e "${YELLOW}Building macOS ARM64...${NC}"
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-X ${VERSION_PKG}.Version=${VERSION} -s -w" -o "${BIN_DIR}/yggradio-darwin-arm64" ./cmd/yggradio
echo -e "${GREEN}✓ macOS ARM64 built${NC}"

echo ""
echo -e "${YELLOW}Building federation server binaries...${NC}"
echo ""

# Federation Server - Linux AMD64
echo -e "${YELLOW}Building Federation Server Linux AMD64...${NC}"
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-X ${VERSION_PKG}.Version=${VERSION} -s -w" -o "${BIN_DIR}/yggradio-federation-server-linux-amd64" ./cmd/yggradio-federation-server
echo -e "${GREEN}✓ Federation Server Linux AMD64 built${NC}"

# Federation Server - Linux ARM64
echo -e "${YELLOW}Building Federation Server Linux ARM64...${NC}"
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-X ${VERSION_PKG}.Version=${VERSION} -s -w" -o "${BIN_DIR}/yggradio-federation-server-linux-arm64" ./cmd/yggradio-federation-server
echo -e "${GREEN}✓ Federation Server Linux ARM64 built${NC}"

# Federation Server - Windows AMD64
echo -e "${YELLOW}Building Federation Server Windows AMD64...${NC}"
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-X ${VERSION_PKG}.Version=${VERSION} -s -w" -o "${BIN_DIR}/yggradio-federation-server-windows-amd64.exe" ./cmd/yggradio-federation-server
echo -e "${GREEN}✓ Federation Server Windows AMD64 built${NC}"

# Federation Server - macOS AMD64
echo -e "${YELLOW}Building Federation Server macOS AMD64...${NC}"
GOOS=darwin GOARCH=amd64 go build -trimpath -ldflags "-X ${VERSION_PKG}.Version=${VERSION} -s -w" -o "${BIN_DIR}/yggradio-federation-server-darwin-amd64" ./cmd/yggradio-federation-server
echo -e "${GREEN}✓ Federation Server macOS AMD64 built${NC}"

# Federation Server - macOS ARM64
echo -e "${YELLOW}Building Federation Server macOS ARM64...${NC}"
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-X ${VERSION_PKG}.Version=${VERSION} -s -w" -o "${BIN_DIR}/yggradio-federation-server-darwin-arm64" ./cmd/yggradio-federation-server
echo -e "${GREEN}✓ Federation Server macOS ARM64 built${NC}"

echo ""
echo -e "${YELLOW}Creating distribution archives...${NC}"

# Create archives for each platform
if command -v tar &> /dev/null; then
    # Linux AMD64
    tar -czf "${DIST_DIR}/yggradio-${VERSION}-linux-amd64.tar.gz" -C "${BIN_DIR}" yggradio-linux-amd64
    echo -e "${GREEN}✓ Created yggradio-${VERSION}-linux-amd64.tar.gz${NC}"

    # Linux ARM64
    tar -czf "${DIST_DIR}/yggradio-${VERSION}-linux-arm64.tar.gz" -C "${BIN_DIR}" yggradio-linux-arm64
    echo -e "${GREEN}✓ Created yggradio-${VERSION}-linux-arm64.tar.gz${NC}"

    # macOS AMD64
    tar -czf "${DIST_DIR}/yggradio-${VERSION}-darwin-amd64.tar.gz" -C "${BIN_DIR}" yggradio-darwin-amd64
    echo -e "${GREEN}✓ Created yggradio-${VERSION}-darwin-amd64.tar.gz${NC}"

    # macOS ARM64
    tar -czf "${DIST_DIR}/yggradio-${VERSION}-darwin-arm64.tar.gz" -C "${BIN_DIR}" yggradio-darwin-arm64
    echo -e "${GREEN}✓ Created yggradio-${VERSION}-darwin-arm64.tar.gz${NC}"

    # Federation Server - Linux AMD64
    tar -czf "${DIST_DIR}/yggradio-federation-server-${VERSION}-linux-amd64.tar.gz" -C "${BIN_DIR}" yggradio-federation-server-linux-amd64
    echo -e "${GREEN}✓ Created yggradio-federation-server-${VERSION}-linux-amd64.tar.gz${NC}"

    # Federation Server - Linux ARM64
    tar -czf "${DIST_DIR}/yggradio-federation-server-${VERSION}-linux-arm64.tar.gz" -C "${BIN_DIR}" yggradio-federation-server-linux-arm64
    echo -e "${GREEN}✓ Created yggradio-federation-server-${VERSION}-linux-arm64.tar.gz${NC}"

    # Federation Server - macOS AMD64
    tar -czf "${DIST_DIR}/yggradio-federation-server-${VERSION}-darwin-amd64.tar.gz" -C "${BIN_DIR}" yggradio-federation-server-darwin-amd64
    echo -e "${GREEN}✓ Created yggradio-federation-server-${VERSION}-darwin-amd64.tar.gz${NC}"

    # Federation Server - macOS ARM64
    tar -czf "${DIST_DIR}/yggradio-federation-server-${VERSION}-darwin-arm64.tar.gz" -C "${BIN_DIR}" yggradio-federation-server-darwin-arm64
    echo -e "${GREEN}✓ Created yggradio-federation-server-${VERSION}-darwin-arm64.tar.gz${NC}"
fi

if command -v zip &> /dev/null; then
    # Windows AMD64
    (cd "${BIN_DIR}" && zip -q "../${DIST_DIR}/yggradio-${VERSION}-windows-amd64.zip" yggradio-windows-amd64.exe)
    echo -e "${GREEN}✓ Created yggradio-${VERSION}-windows-amd64.zip${NC}"

    # Federation Server - Windows AMD64
    (cd "${BIN_DIR}" && zip -q "../${DIST_DIR}/yggradio-federation-server-${VERSION}-windows-amd64.zip" yggradio-federation-server-windows-amd64.exe)
    echo -e "${GREEN}✓ Created yggradio-federation-server-${VERSION}-windows-amd64.zip${NC}"
fi

echo ""
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo -e "${GREEN}Build completed successfully!${NC}"
echo -e "${GREEN}════════════════════════════════════════${NC}"
echo ""
echo -e "Binaries location: ${YELLOW}${BIN_DIR}/${NC}"
echo -e "Archives location: ${YELLOW}${DIST_DIR}/${NC}"
echo ""
echo "Built binaries:"
ls -lh "${BIN_DIR}"/yggradio-* 2>/dev/null | awk '{print "  - " $9 " (" $5 ")"}'
echo ""
echo "Distribution archives:"
ls -lh "${DIST_DIR}"/yggradio-* 2>/dev/null | awk '{print "  - " $9 " (" $5 ")"}'
echo ""
