package git_test

import (
	"testing"

	"agent-orchestrator/git"
)

func TestHasOverlap(t *testing.T) {
	tests := []struct {
		name   string
		a      []string
		b      []string
		expect bool
	}{
		{name: "no overlap", a: []string{"a.go", "b.go"}, b: []string{"c.go"}, expect: false},
		{name: "exact overlap", a: []string{"a.go"}, b: []string{"a.go"}, expect: true},
		{name: "partial overlap", a: []string{"a.go", "b.go"}, b: []string{"b.go", "c.go"}, expect: true},
		{name: "empty a", a: []string{}, b: []string{"a.go"}, expect: false},
		{name: "empty b", a: []string{"a.go"}, b: []string{}, expect: false},
		{name: "both empty", a: []string{}, b: []string{}, expect: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := git.HasOverlap(tc.a, tc.b)
			if got != tc.expect {
				t.Errorf("HasOverlap(%v, %v) = %v, want %v", tc.a, tc.b, got, tc.expect)
			}
		})
	}
}
