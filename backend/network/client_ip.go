package network

import (
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

type ClientIPResolver struct {
	trustedProxies []netip.Prefix
}

func NewClientIPResolver(trustedProxies []string) (*ClientIPResolver, error) {
	prefixes := make([]netip.Prefix, 0, len(trustedProxies))
	for _, value := range trustedProxies {
		prefix, err := parsePrefix(value)
		if err != nil {
			return nil, fmt.Errorf("trusted proxy %q: %w", value, err)
		}

		prefixes = append(prefixes, prefix)
	}

	return &ClientIPResolver{trustedProxies: prefixes}, nil
}

func (r *ClientIPResolver) ClientIP(request *http.Request) string {
	peerText := hostOnly(request.RemoteAddr)

	peer, err := netip.ParseAddr(peerText)
	if err != nil {
		return peerText
	}

	peer = peer.Unmap()
	if !r.trusted(peer) {
		return peerText
	}

	forwardedHeader := strings.TrimSpace(request.Header.Get("X-Forwarded-For"))
	if forwardedHeader != "" {
		forwarded := strings.Split(forwardedHeader, ",")
		for index := len(forwarded) - 1; index >= 0; index-- {
			candidate, parseErr := netip.ParseAddr(strings.TrimSpace(forwarded[index]))
			if parseErr != nil {
				return peerText
			}

			candidate = candidate.Unmap()

			peerText = candidate.String()
			if !r.trusted(candidate) {
				return peerText
			}
		}

		return peerText
	}

	if realIP, parseErr := netip.ParseAddr(strings.TrimSpace(request.Header.Get("X-Real-IP"))); parseErr == nil {
		return realIP.Unmap().String()
	}

	return peerText
}

func (r *ClientIPResolver) trusted(address netip.Addr) bool {
	for _, prefix := range r.trustedProxies {
		if prefix.Contains(address) {
			return true
		}
	}

	return false
}

func parsePrefix(value string) (netip.Prefix, error) {
	value = strings.TrimSpace(value)
	if prefix, err := netip.ParsePrefix(value); err == nil {
		return prefix.Masked(), nil
	}

	address, err := netip.ParseAddr(value)
	if err != nil {
		return netip.Prefix{}, err
	}

	return netip.PrefixFrom(address, address.BitLen()), nil
}

func hostOnly(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return host
	}

	return address
}
