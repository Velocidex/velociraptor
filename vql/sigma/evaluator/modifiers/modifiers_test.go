package modifiers

import (
	"context"
	"fmt"
	"math/rand"
	"testing"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
)

func Test_compareNumeric(t *testing.T) {
	tests := []struct {
		left    interface{}
		right   interface{}
		wantGt  bool
		wantGte bool
		wantLt  bool
		wantLte bool
	}{
		{1, 2, false, false, true, true},
		{1.1, 1.2, false, false, true, true},
		{1, 1.2, false, false, true, true},
		{1.1, 2, false, false, true, true},
		{1, "2", false, false, true, true},
		{"1.1", 1.2, false, false, true, true},
		{"1.1", 1.1, false, true, false, true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.left, tt.right), func(t *testing.T) {
			gotGt, gotGte, gotLt, gotLte, err := compareNumeric(tt.left, tt.right)
			if err != nil {
				t.Errorf("compareNumeric() error = %v", err)
				return
			}
			if gotGt != tt.wantGt {
				t.Errorf("compareNumeric() gotGt = %v, want %v", gotGt, tt.wantGt)
			}
			if gotGte != tt.wantGte {
				t.Errorf("compareNumeric() gotGte = %v, want %v", gotGte, tt.wantGte)
			}
			if gotLt != tt.wantLt {
				t.Errorf("compareNumeric() gotLt = %v, want %v", gotLt, tt.wantLt)
			}
			if gotLte != tt.wantLte {
				t.Errorf("compareNumeric() gotLte = %v, want %v", gotLte, tt.wantLte)
			}
		})
	}
}

func BenchmarkContains(b *testing.B) {
	needle := "abcdefg"

	ctx := context.Background()
	scope := vql_subsystem.MakeScope()

	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	haystack := make([]rune, 1_000_000)
	for i := range haystack {
		haystack[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	haystackString := string(haystack)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := contains{}.Matches(
			ctx, scope, string(haystackString), needle)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkContainsCS(b *testing.B) {
	needle := "abcdefg"

	ctx := context.Background()
	scope := vql_subsystem.MakeScope()

	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	haystack := make([]rune, 1_000_000)
	for i := range haystack {
		haystack[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	haystackString := string(haystack)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := containsCS{}.Matches(
			ctx, scope, string(haystackString), needle)
		if err != nil {
			b.Fatal(err)
		}
	}
}
