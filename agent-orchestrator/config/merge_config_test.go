package config

import "testing"

func TestMergeConfigDefaults(t *testing.T) {
	// Unset (nil) → both default to true.
	var m MergeConfig
	if !m.ShouldSquash() {
		t.Error("default squash should be true")
	}
	if !m.ShouldDeleteBranch() {
		t.Error("default delete_branch should be true")
	}

	// Explicit false is honoured.
	f := false
	m2 := MergeConfig{Squash: &f, DeleteBranch: &f}
	if m2.ShouldSquash() {
		t.Error("explicit squash=false should disable squashing")
	}
	if m2.ShouldDeleteBranch() {
		t.Error("explicit delete_branch=false should keep the branch")
	}
}
