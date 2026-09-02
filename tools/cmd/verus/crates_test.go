package main

import (
	"path/filepath"
	"testing"
)

func TestParseCargoToml(t *testing.T) {
	on := `[package]
name = "store"
version.workspace = true

[package.metadata.verus]
verify = true
`
	if n, v := parseCargoToml(on); n != "store" || !v {
		t.Errorf("verify-enabled crate read as name=%q verify=%v", n, v)
	}

	// The section has to be the right one. A `verify = true` under any other
	// table is not the Verus key, and treating it as one would silently widen
	// the verification matrix -- the denominator every "N of M crates
	// reported" check is measured against.
	off := `[package]
name = "server"

[features]
verify = true
`
	if n, v := parseCargoToml(off); n != "server" || v {
		t.Errorf("a verify key outside [package.metadata.verus] was counted: name=%q verify=%v", n, v)
	}
}

// The Rust corner's verification matrix is read out of the tree, so this test
// is also the record of what that matrix currently is: five crates, and
// crates/server -- the trusted transport shim, the analogue of Go's
// internal/httpshim -- deliberately absent (F012, TCB.md).
func TestVerifyEnabledCratesOfTheRealTree(t *testing.T) {
	dir, err := filepath.Abs("../../../impls/rust")
	if err != nil {
		t.Fatal(err)
	}
	cs, err := verifyEnabledCrates(dir)
	if err != nil {
		t.Skipf("impls/rust not readable from here: %v", err)
	}
	got := map[string]string{}
	for _, c := range cs {
		got[c.Name] = c.Rel
	}
	for _, want := range []string{"clock", "ids", "domain", "store", "service"} {
		if _, ok := got[want]; !ok {
			t.Errorf("crate %s is no longer verify-enabled; the R4 denominator changed", want)
		}
	}
	if _, ok := got["server"]; ok {
		t.Error("crates/server has become verify-enabled; TCB.md's trusted-shim split has changed and F012 needs revisiting")
	}
	if len(cs) != 5 {
		t.Errorf("verification matrix is %d crates, was 5: %v", len(cs), got)
	}
}
