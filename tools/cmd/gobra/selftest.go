package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// selfTest is the canary's own canary.
//
// GOAL.md standing rule 2: no gate is trusted until it has been shown to
// fail. A negation-canary sweep that reported REFUTABLE for everything would
// look identical whether it was working or silently mis-substituting, so the
// sweep runs this first: take one clause known to be live, make it vacuous the
// way F013's Kotlin obligations were made vacuous -- render the code after a
// point infeasible -- and require the sweep to notice.
//
// Expected, and the reason this is a gate rather than a comment:
//
//	control clause, as shipped         -> REFUTABLE
//	same clause behind `assume false`  -> VACUOUS
//
// If the second comes back REFUTABLE the sweep cannot see vacuity at all and
// every REFUTABLE it printed is worthless.
func selfTest(implDir string) error {
	all, err := allClauses(implDir)
	if err != nil {
		return err
	}
	var ctrl *clause
	for i := range all {
		c := all[i]
		if c.File == "internal/clock/clock.go" && c.Member == "(*Logical).Tick" && c.Kind == kindFunctional {
			ctrl = &all[i]
			break
		}
	}
	if ctrl == nil {
		return fmt.Errorf("self-test control clause (*Logical).Tick not found")
	}

	fmt.Fprintf(os.Stderr, "self-test control: %s:%d  %s\n", ctrl.File, ctrl.StartLine, ctrl.Text)

	live := runCanary(implDir, *ctrl, 6*time.Minute)
	fmt.Fprintf(os.Stderr, "  as shipped               -> %s\n", live.Verdict)

	dead, err := runCanaryInfeasible(implDir, *ctrl)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "  behind `assume false`    -> %s\n", dead.Verdict)

	if live.Verdict != refutable {
		return fmt.Errorf("self-test: control clause came back %s, expected REFUTABLE -- "+
			"the sweep is not substituting what it thinks it is", live.Verdict)
	}
	if dead.Verdict != vacuous {
		return fmt.Errorf("self-test: a deliberately unreachable obligation came back %s, "+
			"expected VACUOUS -- the sweep cannot detect vacuity, so no REFUTABLE it "+
			"reports means anything", dead.Verdict)
	}
	fmt.Fprintln(os.Stderr, "  self-test PASSED: the sweep distinguishes a live obligation from a vacuous one")
	return nil
}

// runCanaryInfeasible runs the canary with the clause's member made
// unreachable, which is what F013's undischargeable checkcast did by accident.
func runCanaryInfeasible(implDir string, c clause) (canaryResult, error) {
	return runCanaryInfeasibleIsolating(implDir, c, nil, 6*time.Minute)
}

// runCanaryInfeasibleIsolating is the per-clause control run.
//
// Standing rule 2 applied to the probe itself: a canary that reports REFUTABLE
// for a clause has proved nothing unless the same canary, pointed at a copy of
// the member whose exit really is unreachable, reports VACUOUS. The sweep's
// self-test establishes that once, on (*clock.Logical).Tick. On a member where
// the sweep's default shape does not terminate the question has to be asked
// again in whatever shape did terminate, on that member -- which is what this
// is for.
func runCanaryInfeasibleIsolating(implDir string, c clause, siblings []clause, budget time.Duration) (canaryResult, error) {
	ws, err := newWorkspace(implDir)
	if err != nil {
		return canaryResult{}, err
	}
	defer ws.close()
	path := filepath.Join(ws.module, c.File)
	if err := substitute(path, c); err != nil {
		return canaryResult{}, err
	}
	if len(siblings) > 0 {
		if err := elide(path, siblings); err != nil {
			return canaryResult{}, err
		}
	}
	if err := injectAssumeFalse(path, c.Member); err != nil {
		return canaryResult{}, err
	}
	res, err := runGobra(ws, []string{c.Pkg}, "", budget)
	r := canaryResult{Clause: c, Elapsed: res.Elapsed, ElapsedS: res.Elapsed.Seconds()}
	if errors.Is(err, errTimeout) {
		r.Verdict = timedOut
		return r, nil
	}
	if err != nil {
		return r, err
	}
	if res.Total == 0 {
		r.Verdict = vacuous
	} else {
		r.Verdict = refutable
	}
	return r, nil
}

// injectAssumeFalse puts `assume false` at the top of a member's body, so
// every statement after it -- and therefore the postcondition -- sits on an
// infeasible path.
func injectAssumeFalse(path, member string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	for i, ln := range lines {
		s := strings.TrimSpace(ln)
		var name string
		if m := reRecvName.FindStringSubmatch(s); m != nil {
			name = "(*" + m[1] + ")." + m[2]
		} else if m := reFuncDecl.FindStringSubmatch(s); m != nil {
			name = m[1]
		}
		if name != member {
			continue
		}
		out := append([]string{}, lines[:i+1]...)
		out = append(out, "\t// @ assume false")
		out = append(out, lines[i+1:]...)
		return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
	}
	return fmt.Errorf("%s: member %s not found", path, member)
}
