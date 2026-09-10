package executor

import (
	"fmt"
	"math/rand"
	"strings"
)

// PoolIdentity represents the emulated identity of a pool client.
// Each pool client presents a unique hostname and FQDN to the server
// while all other client information (OS, platform, architecture,
// interfaces etc.) reflects the real host.
type PoolIdentity struct {
	Hostname string
	Fqdn     string
}

// Word lists used to generate codename-style hostnames in the style
// of adjective-noun combinations (e.g. "brave-falcon",
// "cosmic-orbit"). The lists are deliberately platform agnostic so
// the generated names look plausible on any operating system.
var (
	pool_adjectives = []string{
		"brave", "swift", "cosmic", "golden", "silver", "crimson",
		"emerald", "azure", "violet", "amber", "mystic", "silent",
		"raging", "gentle", "fierce", "noble", "royal", "ancient",
		"modern", "digital", "quantum", "stellar", "lunar", "solar",
		"shadowy", "phantom", "thunderous", "stormy", "frosty",
		"blazing", "radiant", "hidden", "secret", "sacred", "wild",
		"calm", "bright", "dark", "deep", "quick", "strong", "wise",
		"lucky", "happy", "sunny", "misty", "rocky", "sandy", "grassy",
		"woody", "icy", "fiery", "electric", "magnetic", "atomic",
		"orbital", "galactic", "nebular", "prismatic", "echoing",
		"pulsing",
		"cyber", "neon", "plasma", "photon", "binary", "chrome",
		"silicon", "fractal", "vector", "pixel", "glitch", "static",
		"sonic", "laser", "turbo", "hyper", "stealth", "tactical",
		"infinite", "eternal", "arcane", "runic", "eldritch", "null",
		"void", "zero", "temporal", "kinetic", "synthetic",
		"holographic",
	}

	pool_nouns = []string{
		"falcon", "eagle", "hawk", "raven", "owl", "wolf", "fox",
		"bear", "lion", "tiger", "panther", "leopard", "jaguar",
		"cheetah", "lynx", "puma", "cobra", "viper", "python",
		"dragon", "phoenix", "griffin", "unicorn", "pegasus", "hydra",
		"kraken", "leviathan", "behemoth", "colossus", "titan",
		"wizard", "knight", "samurai", "ninja", "ranger", "hunter",
		"scout", "sentinel", "guardian", "defender", "warrior",
		"champion", "hero", "legend", "myth", "saga", "odyssey",
		"voyage", "journey", "quest", "compass", "beacon",
		"lighthouse", "anchor", "horizon", "summit", "peak", "ridge",
		"canyon", "river", "ocean",
		"robot", "android", "cyborg", "droid", "mech",
		"singularity", "wormhole", "starship", "rocket", "probe",
		"comet", "meteor", "asteroid", "nebula", "galaxy", "eclipse",
		"aurora", "quasar", "pulsar", "nova", "zenith", "kernel",
		"daemon", "proxy", "cipher", "firewall", "matrix", "node",
		"basilisk", "chimera", "cerberus", "wyvern", "drake", "golem",
		"paladin", "berserker", "gladiator", "archer", "renegade",
		"nomad",
	}
)

// GeneratePoolIdentities returns count unique identities for pool
// clients. Hostnames are generated as adjective-noun combinations
// (e.g. "brave-falcon"). If the number of clients exceeds the number
// of available combinations a random numeric suffix is appended to
// keep every hostname unique.
//
// The FQDN reuses the domain portion of the real host's FQDN so the
// emulated clients appear to belong to the same network as the host
// running the pool client (e.g. "brave-falcon.example.com" when the
// host is "myhost.example.com").
func GeneratePoolIdentities(count int, real_fqdn string) []PoolIdentity {
	domain := deriveDomain(real_fqdn)
	used := make(map[string]bool)
	result := make([]PoolIdentity, 0, count)

	for i := 0; i < count; i++ {
		hostname := generateUniqueHostname(used)
		result = append(result, PoolIdentity{
			Hostname: hostname,
			Fqdn:     hostname + "." + domain,
		})
	}

	return result
}

// deriveDomain extracts the domain portion of a fully qualified
// domain name. For example "myhost.example.com" returns
// "example.com". If the FQDN has no domain portion (e.g. a bare
// hostname) we fall back to "local" so generated FQDNs always look
// like proper domain names.
func deriveDomain(real_fqdn string) string {
	parts := strings.SplitN(real_fqdn, ".", 2)
	if len(parts) == 2 && parts[1] != "" {
		return parts[1]
	}
	return "local"
}

func generateUniqueHostname(used map[string]bool) string {
	for {
		hostname := fmt.Sprintf("%s-%s",
			pool_adjectives[rand.Intn(len(pool_adjectives))],
			pool_nouns[rand.Intn(len(pool_nouns))])

		if !used[hostname] {
			used[hostname] = true
			return hostname
		}

		// The base combination is already taken - disambiguate with a
		// random numeric suffix.
		hostname = fmt.Sprintf("%s-%d", hostname, rand.Intn(999)+1)
		if !used[hostname] {
			used[hostname] = true
			return hostname
		}
	}
}
