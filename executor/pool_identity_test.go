package executor

import (
	"testing"
)

func TestGeneratePoolIdentities(t *testing.T) {
	// Generate a large number of identities and make sure they are
	// all unique.
	count := 10000
	identities := GeneratePoolIdentities(count, "myhost.example.com")

	if len(identities) != count {
		t.Fatalf("Expected %v identities, got %v", count, len(identities))
	}

	seen := make(map[string]bool)
	for _, id := range identities {
		if id.Hostname == "" {
			t.Fatalf("Empty hostname generated")
		}
		// The FQDN reuses the domain of the real host.
		if id.Fqdn != id.Hostname+".example.com" {
			t.Fatalf("Fqdn %v does not match hostname %v", id.Fqdn, id.Hostname)
		}
		if seen[id.Hostname] {
			t.Fatalf("Duplicate hostname generated: %v", id.Hostname)
		}
		seen[id.Hostname] = true
	}
}

func TestGeneratePoolIdentitiesSmall(t *testing.T) {
	// Even a small number of identities must be unique.
	identities := GeneratePoolIdentities(100, "myhost")

	seen := make(map[string]bool)
	for _, id := range identities {
		if id.Fqdn != id.Hostname+".local" {
			t.Fatalf("Fqdn %v does not match hostname %v", id.Fqdn, id.Hostname)
		}
		if seen[id.Hostname] {
			t.Fatalf("Duplicate hostname generated: %v", id.Hostname)
		}
		seen[id.Hostname] = true
	}
}

func TestDeriveDomain(t *testing.T) {
	table := []struct {
		name     string
		fqdn     string
		expected string
	}{
		{"fully qualified", "myhost.example.com", "example.com"},
		{"bare hostname", "myhost", "local"},
		{"local domain", "myhost.local", "local"},
		{"empty", "", "local"},
	}

	for _, tt := range table {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveDomain(tt.fqdn)
			if result != tt.expected {
				t.Fatalf("deriveDomain(%q) = %q, want %q",
					tt.fqdn, result, tt.expected)
			}
		})
	}
}
