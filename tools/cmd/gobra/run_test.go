package main

import "testing"

// A package that exceeds --packageTimeout prints its termination line and
// then, on the very next line, "Gobra has found 0 error(s)". Anything reading
// the count alone scores a verification that never ran as a clean pass. In a
// negation sweep that inverts into the worst possible answer: the negation
// "verified", so the obligation is reported VACUOUS. Detection has to key on
// the termination line, and has to be tight enough not to fire on the stack
// trace that a malformed --packageTimeout argument produces.
func TestTerminatedDetection(t *testing.T) {
	timedOutTranscript := `02:32:05.761 [main] ERROR viper.gobra.Gobra - The verification of package /w/internal/store - store got terminated after 1 second
02:32:05.767 [main] INFO viper.gobra.Gobra - Gobra has found 0 error(s)`
	if !gobraTimedOut(timedOutTranscript) {
		t.Error("a timed-out package was not detected; its `0 error(s)` would read as a pass")
	}

	// Gobra rejects Go's "6m0s" duration with a NumberFormatException whose
	// trace names packageTimeoutDuration. That is a crash, not a timeout, and
	// must surface as one.
	badArgTranscript := `ERROR viper.gobra.GobraRunner$ - For input string: "6m0"
java.lang.NumberFormatException: For input string: "6m0"
	at viper.gobra.frontend.ScallopGobraConfig.packageTimeoutDuration(Config.scala:665)`
	if gobraTimedOut(badArgTranscript) {
		t.Error("a NumberFormatException on the timeout argument was misread as a timeout")
	}

	clean := `INFO viper.gobra.Gobra - Gobra found no errors
INFO viper.gobra.Gobra - Gobra has found 0 error(s)`
	if gobraTimedOut(clean) {
		t.Error("a clean run was misread as a timeout")
	}
}
