package network

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// activeAddress represents a resolvable local address for network binding.
type activeAddress struct {
	addr *net.TCPAddr
}

// ipState tracks the status of a configured bind address.
type ipState struct {
	address     *activeAddress
	isExhausted bool
	exhaustedAt time.Time
}

// IPRotator manages a pool of bind addresses, handling rotation and rate-limit fallback.
type IPRotator struct {
	mu            sync.RWMutex
	addresses     []*ipState
	currentIndex  int
	exhaustionTTL time.Duration
	lastUsed      *net.TCPAddr
}

// NewIPRotator creates and initializes a new IPRotator.
// It resolves and validates all provided addresses.
func NewIPRotator(bindAddresses string, exhaustionTTL time.Duration) (*IPRotator, error) {
	if strings.TrimSpace(bindAddresses) == "" {
		return nil, fmt.Errorf("bind addresses cannot be empty")
	}

	parts := strings.Split(bindAddresses, ",")
	if len(parts) == 0 {
		return nil, fmt.Errorf("no valid bind addresses found")
	}

	rotator := &IPRotator{
		addresses:     make([]*ipState, 0, len(parts)),
		exhaustionTTL: exhaustionTTL,
	}

	for _, part := range parts {
		trimmedPart := strings.TrimSpace(part)
		if trimmedPart == "" {
			continue
		}
		tcpAddr, err := resolveBindAddr(trimmedPart)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve bind address '%s': %w", trimmedPart, err)
		}
		rotator.addresses = append(rotator.addresses, &ipState{
			address: &activeAddress{addr: tcpAddr},
		})
	}

	if len(rotator.addresses) == 0 {
		return nil, fmt.Errorf("no usable addresses could be resolved from '%s'", bindAddresses)
	}

	return rotator, nil
}

// GetNextAvailableAddress finds the next non-exhausted address for use, cycling through the list.
func (r *IPRotator) GetNextAvailableAddress() (*net.TCPAddr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.addresses) == 0 {
		return nil, fmt.Errorf("no addresses configured in rotator")
	}

	// Un-exhaust any IPs whose TTL has expired.
	for _, state := range r.addresses {
		if state.isExhausted && time.Since(state.exhaustedAt) > r.exhaustionTTL {
			state.isExhausted = false
		}
	}

	// Starting from the current index, find the next available address.
	for i := 0; i < len(r.addresses); i++ {
		idx := (r.currentIndex + i) % len(r.addresses)
		if !r.addresses[idx].isExhausted {
			addr := r.addresses[idx].address.addr
			r.currentIndex = (idx + 1) % len(r.addresses)
			r.lastUsed = addr
			return addr, nil
		}
	}

	return nil, fmt.Errorf("all available IP addresses are currently rate-limited")
}

// MarkCurrentAddressAsExhausted flags the most recently used IP as rate-limited.
func (r *IPRotator) MarkCurrentAddressAsExhausted() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.lastUsed == nil {
		return
	}

	for _, state := range r.addresses {
		if state.address.addr.String() == r.lastUsed.String() {
			state.isExhausted = true
			state.exhaustedAt = time.Now()
			break
		}
	}
}

// resolveBindAddr takes a string that can be an IP address or an interface name
// and returns a resolvable *net.TCPAddr.
func resolveBindAddr(addrOrInterface string) (*net.TCPAddr, error) {
	ip := net.ParseIP(addrOrInterface)
	if ip != nil {
		return &net.TCPAddr{IP: ip}, nil
	}

	iface, err := net.InterfaceByName(addrOrInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to find network interface '%s': %w", addrOrInterface, err)
	}

	addrs, err := iface.Addrs()
	if err != nil || len(addrs) == 0 {
		return nil, fmt.Errorf("interface '%s' has no usable addresses", addrOrInterface)
	}

	for _, addr := range addrs {
		var ip net.IP
		if ipNet, ok := addr.(*net.IPNet); ok {
			ip = ipNet.IP
		} else if ipAddr, ok := addr.(*net.IPAddr); ok {
			ip = ipAddr.IP
		}

		if ip != nil && ip.To4() != nil && !ip.IsLoopback() {
			return &net.TCPAddr{IP: ip}, nil
		}
	}

	return nil, fmt.Errorf("no usable IPv4 address found for interface '%s'", addrOrInterface)
}
