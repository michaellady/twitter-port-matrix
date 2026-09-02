package main

import (
	"testing"
	"time"
)

// The verdict sentence carries Gobra's own count verbatim and starts with the
// exact prefix calibrate anchors on.
func TestR4VerdictLine(t *testing.T) {
	if got := r4Verdict(0, 5, 59*time.Second); got != "R4 PASSED: Gobra has found 0 error(s) over 5 package(s)   [59s]" {
		t.Errorf("pass: %q", got)
	}
	if got := r4Verdict(2, 5, 61*time.Second); got != "R4 FAILED: Gobra has found 2 error(s) over 5 package(s)   [1m1s]" {
		t.Errorf("fail: %q", got)
	}
}
