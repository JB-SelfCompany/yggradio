package utils

import (
	"fmt"
	"net"
	"sort"
)

// YggdrasilAddress represents a Yggdrasil IPv6 address with priority
type YggdrasilAddress struct {
	IP       net.IP
	Priority int // Lower number = higher priority
}

// DetectYggdrasilAddresses detects all Yggdrasil IPv6 addresses from network interfaces
// Returns addresses sorted by priority:
// - Priority 1: Full addresses (200::/7 range with /7 or /128 prefix)
// - Priority 2: Subnet addresses (300::/7 range)
func DetectYggdrasilAddresses() ([]YggdrasilAddress, error) {
	// Get all network interfaces
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to get network interfaces: %w", err)
	}

	var addresses []YggdrasilAddress

	// Scan all interfaces for Yggdrasil addresses
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP
			if ip.To4() != nil {
				continue // Skip IPv4
			}

			// Check if it's a Yggdrasil address (200::/7 or 300::/7)
			// First byte should be 0x02 or 0x03
			if len(ip) == 16 && (ip[0] == 0x02 || ip[0] == 0x03) {
				priority := 2 // Default: subnet address (300::/7)

				// Priority 1: Full Yggdrasil address (200::/7)
				if ip[0] == 0x02 {
					priority = 1
				}

				addresses = append(addresses, YggdrasilAddress{
					IP:       ip,
					Priority: priority,
				})
			}
		}
	}

	if len(addresses) == 0 {
		return nil, fmt.Errorf("no Yggdrasil IPv6 addresses found on any interface (addresses must be in 200::/7 or 300::/7 range)")
	}

	// Sort by priority (lower number = higher priority)
	sort.Slice(addresses, func(i, j int) bool {
		return addresses[i].Priority < addresses[j].Priority
	})

	return addresses, nil
}

// IsPortAvailable checks if a port is available on a specific IPv6 address
func IsPortAvailable(ip net.IP, port int) bool {
	addr := fmt.Sprintf("[%s]:%d", ip.String(), port)
	listener, err := net.Listen("tcp6", addr)
	if err != nil {
		return false
	}
	listener.Close()
	return true
}

// FindAvailableAddress finds the first available Yggdrasil address:port combination
// Tries addresses in priority order (200::/7 addresses first, then 300::/7)
func FindAvailableAddress(port int) (net.IP, error) {
	addresses, err := DetectYggdrasilAddresses()
	if err != nil {
		return nil, err
	}

	// Try each address in priority order
	for _, addr := range addresses {
		if IsPortAvailable(addr.IP, port) {
			return addr.IP, nil
		}
	}

	return nil, fmt.Errorf("port %d is not available on any Yggdrasil address", port)
}
