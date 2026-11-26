//go:build darwin

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCanonicalPathFoldsCaseAliasesOnCaseInsensitiveVolumes(t *testing.T) {
	base := t.TempDir()
	writeSized(t, filepath.Join(base, "Tool", "f"), 4)
	if _, err := os.Stat(filepath.Join(base, "tool")); err != nil {
		t.Skip("volume is case-sensitive")
	}
	upper, lower := canonicalPath(filepath.Join(base, "Tool")), canonicalPath(filepath.Join(base, "tool"))
	if upper != lower || filepath.Base(lower) != "Tool" {
		t.Fatalf("case aliases resolve to %q and %q; want the on-disk spelling for both", upper, lower)
	}
	nested := canonicalPath(filepath.Join(base, "TOOL", "f"))
	if !isWithin(nested, upper) {
		t.Fatalf("%q is not recognised as inside %q", nested, upper)
	}
}
