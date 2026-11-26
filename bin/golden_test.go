package main

import "testing"

func TestParseFixtureYAMLFields(t *testing.T) {
	fixture, err := parseFixture([]byte(`Queries:
  - SELECT * FROM scope()
Parameters: {Example: value}
Transforms:
  - Search: before
    Replace: after
ConfigMerges: [merge]
ConfigPatches: [patch]
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(fixture.Queries) != 1 || fixture.Queries[0].Query != "SELECT * FROM scope()" {
		t.Fatalf("unexpected queries: %#v", fixture.Queries)
	}
	if fixture.Parameters["Example"] != "value" ||
		len(fixture.Transforms) != 1 || fixture.Transforms[0].Search != "before" ||
		fixture.Transforms[0].Replace != "after" || len(fixture.ConfigMerges) != 1 ||
		len(fixture.ConfigPatches) != 1 {
		t.Fatalf("unexpected fixture: %#v", fixture)
	}
}
