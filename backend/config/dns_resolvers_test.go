package config

import "testing"

func TestApplyEnvUsesPublicDNSResolversByDefault(t *testing.T) {
	t.Setenv("CERTVAULT_ACME_DNS_RESOLVERS", "")

	configuration := Config{}

	if err := applyEnv(&configuration); err != nil {
		t.Fatal(err)
	}

	expected := []string{"1.1.1.1:53", "1.0.0.1:53"}
	assertDNSResolvers(t, configuration.ACME.DNSResolvers, expected)
}

func TestApplyEnvOverridesDNSResolvers(t *testing.T) {
	t.Setenv("CERTVAULT_ACME_DNS_RESOLVERS", "9.9.9.9:53, 8.8.8.8")

	configuration := Config{ACME: ACME{DNSResolvers: []string{"192.168.1.1:53"}}}

	if err := applyEnv(&configuration); err != nil {
		t.Fatal(err)
	}

	expected := []string{"9.9.9.9:53", "8.8.8.8"}
	assertDNSResolvers(t, configuration.ACME.DNSResolvers, expected)
}

func assertDNSResolvers(t *testing.T, actual, expected []string) {
	t.Helper()

	if len(actual) != len(expected) {
		t.Fatalf("DNS resolvers = %#v, want %#v", actual, expected)
	}

	for index := range expected {
		if actual[index] != expected[index] {
			t.Fatalf("DNS resolvers = %#v, want %#v", actual, expected)
		}
	}
}
