package handlers

import (
	"net"
	"strings"
)

// extractIPv6 extracts IPv6 address from RemoteAddr
// Format: "[ipv6]:port"
func extractIPv6(remoteAddr string) string {
	// Use net.SplitHostPort for proper IPv6 handling
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}

	// If SplitHostPort fails, try manual extraction for legacy format
	// IPv6 format: [address]:port
	if len(remoteAddr) > 0 && remoteAddr[0] == '[' {
		endIdx := strings.Index(remoteAddr, "]")
		if endIdx > 0 {
			return remoteAddr[1:endIdx]
		}
	}

	// Return as-is if no port separator found
	return remoteAddr
}

// extractIPv6Subnet extracts /64 subnet from IPv6 address
// Example: 201:be28:cf55:3c9:f517:1d08:4699:5ce7 -> 201:be28:cf55:3c9::
func extractIPv6Subnet(ipv6 string) string {
	ip := net.ParseIP(ipv6)
	if ip == nil {
		return ""
	}

	// Ensure it's IPv6
	ip = ip.To16()
	if ip == nil {
		return ""
	}

	// Create /64 subnet mask (first 64 bits)
	mask := net.CIDRMask(64, 128)
	subnet := &net.IPNet{
		IP:   ip.Mask(mask),
		Mask: mask,
	}

	return subnet.String()
}
