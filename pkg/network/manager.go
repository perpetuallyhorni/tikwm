package network

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"
)

var (
	// globalRotator is the instance that manages IP addresses.
	globalRotator *IPRotator
)

// InitManager initializes the global network manager with IP rotation capabilities.
// It replaces the http.DefaultTransport with a custom one if bind addresses are provided.
func InitManager(bindAddresses string) error {
	if strings.TrimSpace(bindAddresses) == "" {
		// No custom binding, use default transport.
		return nil
	}

	var err error
	// The per-IP daily limit is 24 hours.
	globalRotator, err = NewIPRotator(bindAddresses, 24*time.Hour)
	if err != nil {
		return err
	}

	// Create a transport that gets its dialer dynamically from the rotator.
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			nextAddr, err := globalRotator.GetNextAvailableAddress()
			if err != nil {
				return nil, err // All IPs are exhausted
			}
			dialer := &net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
				LocalAddr: nextAddr,
			}
			return dialer.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Set the custom transport as the default for all HTTP clients.
	http.DefaultClient = &http.Client{Transport: transport}
	return nil
}

// MarkCurrentAddressAsExhausted signals the global rotator to mark the last-used IP as exhausted.
func MarkCurrentAddressAsExhausted() {
	if globalRotator != nil {
		globalRotator.MarkCurrentAddressAsExhausted()
	}
}
