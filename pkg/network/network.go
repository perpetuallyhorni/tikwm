package network

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

var globalTransport http.RoundTripper

// SetGlobalTransport sets the transport to be used for all HTTP clients.
func SetGlobalTransport(transport http.RoundTripper) {
	globalTransport = transport
	http.DefaultClient = &http.Client{Transport: transport}
}

// GetGlobalTransport returns the currently configured global HTTP transport.
// If none is set, it returns http.DefaultTransport.
func GetGlobalTransport() http.RoundTripper {
	if globalTransport != nil {
		return globalTransport
	}
	return http.DefaultTransport
}

// // resolveBindAddr takes a string that can be an IP address or an interface name
// // and returns a resolvable *net.TCPAddr.
// func resolveBindAddr(addrOrInterface string) (*net.TCPAddr, error) {
// 	// First, try to parse as an IP address directly.
// 	ip := net.ParseIP(addrOrInterface)
// 	if ip != nil {
// 		return &net.TCPAddr{IP: ip}, nil
// 	}

// 	// If it's not a valid IP, assume it's an interface name.
// 	iface, err := net.InterfaceByName(addrOrInterface)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to find network interface '%s': %w", addrOrInterface, err)
// 	}

// 	addrs, err := iface.Addrs()
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to get addresses for interface '%s': %w", addrOrInterface, err)
// 	}
// 	if len(addrs) == 0 {
// 		return nil, fmt.Errorf("network interface '%s' has no addresses", addrOrInterface)
// 	}

// 	// Find the first usable IPv4 address on the interface.
// 	for _, addr := range addrs {
// 		var ip net.IP
// 		// The addr is of type net.Addr, which can be *net.IPNet or *net.IPAddr.
// 		// We need to check and extract the IP.
// 		if ipNet, ok := addr.(*net.IPNet); ok {
// 			ip = ipNet.IP
// 		} else if ipAddr, ok := addr.(*net.IPAddr); ok {
// 			ip = ipAddr.IP
// 		}

// 		if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
// 			return &net.TCPAddr{IP: ip}, nil
// 		}
// 	}

// 	return nil, fmt.Errorf("no usable IPv4 address found for interface '%s'", addrOrInterface)
// }

// NewHTTPTransport creates a new http.Transport that binds to the specified local address.
func NewHTTPTransport(bindAddr string) (*http.Transport, error) {
	if strings.TrimSpace(bindAddr) == "" {
		return nil, fmt.Errorf("bind address cannot be empty")
	}

	localAddr, err := resolveBindAddr(bindAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve bind address '%s': %w", bindAddr, err)
	}

	// Create a custom dialer with the local address set.
	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		LocalAddr: localAddr,
	}

	// Create a transport that uses our custom dialer.
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Verify that the transport can establish a connection.
	testClient := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	resp, err := testClient.Get("https://api.github.com") // A reliable, neutral endpoint.
	if err != nil {
		return nil, fmt.Errorf("failed to verify connection with bind address %s: %w", localAddr.IP, err)
	}
	_ = resp.Body.Close()

	return transport, nil
}
