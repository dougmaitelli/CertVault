package network

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestClientIPIgnoresHeadersFromUntrustedPeer(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.8")

	if actual := resolver.ClientIP(request); actual != "192.0.2.10" {
		t.Fatalf("ClientIP() = %q", actual)
	}
}

func TestClientIPUsesTrustedProxyChain(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8", "192.168.1.10"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	request.RemoteAddr = "10.0.0.5:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.7, 192.168.1.10")

	if actual := resolver.ClientIP(request); actual != "198.51.100.7" {
		t.Fatalf("ClientIP() = %q", actual)
	}
}

func TestNewClientIPResolverRejectsInvalidNetwork(t *testing.T) {
	if _, err := NewClientIPResolver([]string{"not-a-network"}); err == nil {
		t.Fatal("invalid trusted proxy was accepted")
	}
}

func TestClientIPRejectsMalformedForwardedChain(t *testing.T) {
	resolver, err := NewClientIPResolver([]string{"10.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequestWithContext(context.Background(), "GET", "/", nil)
	request.RemoteAddr = "10.0.0.5:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.7, invalid")

	if actual := resolver.ClientIP(request); actual != "10.0.0.5" {
		t.Fatalf("ClientIP() = %q", actual)
	}
}
