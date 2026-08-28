package main

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/michaellady/twitter-port-matrix/tools/internal/implrun"
)

// HARD REQUIREMENT 3 -- cost must be comparable.
//
// Raw seconds are not a rung's cost. Every rung here relaunches the
// implementation: replay once for the whole corpus, diffrun once per trace so
// no state leaks between traces, proptest once per property per round -- which
// at the defaults is around 72 launches for a single R2 run. Each launch is a
// warm build check plus a process start plus a health wait, and Go and Rust do
// not pay the same for that. So "R2 costs 45s on Rust and 110s on Go" can be
// entirely a statement about process startup and say nothing about R2.
//
// The fix is not to guess a correction factor. The floor is MEASURED, per
// corner, by doing exactly what a rung does per launch and timing it. The table
// then reports two numbers side by side and labels them:
//
//	wall    what the rung actually took. What you pay.
//	tool    wall - launches*floor. What the rung costs once the process floor
//	        is removed. Comparable across corners; comparable across rungs only
//	        to the extent that the remaining work is the same kind of work.
//
// Neither is "the" cost, and the report never prints one without the other.
//
// Two honesty constraints on the tool column:
//
//   - It is only computed for CLEAN runs. A rung that stops at its first
//     mismatch launches an unknown, smaller number of servers, so a killed
//     mutant's wall time is not comparable with a survivor's and subtracting a
//     launch count nobody measured would invent a number. Killed cells print
//     wall only.
//
//   - The floor is the mean of a few samples on a machine also running the
//     sweep. It is a good enough correction to stop startup dominating the
//     comparison; it is not a benchmark.

// A Floor is one corner's measured per-launch cost.
type Floor struct {
	Impl     string  `json:"impl"`
	Samples  int     `json:"samples"`
	LaunchMS float64 `json:"launch_ms"` // mean of build-check + start + health wait
	BuildMS  float64 `json:"build_ms"`  // the build-check share of it
	MinMS    float64 `json:"min_ms"`
	MaxMS    float64 `json:"max_ms"`
}

// measureFloor times what a rung pays for one server.
//
// It deliberately includes the build command. Every rung's launch path runs the
// implementation's declared build before starting the process -- warm, so it is
// a cache check rather than a compile, but it is not free and it is paid once
// per launch. Timing only the process start would understate R1 and R2 by the
// build-check cost times their launch counts, and that is precisely the term
// that differs most between a Go corner and a Rust corner.
func measureFloor(root, registryPath, impl string, samples int) (Floor, error) {
	reg, err := implrun.LoadRegistry(underRoot(root, registryPath))
	if err != nil {
		return Floor{}, err
	}
	spec, err := reg.Get(impl)
	if err != nil {
		return Floor{}, err
	}
	if !filepath.IsAbs(spec.Dir) {
		spec.Dir = filepath.Join(root, spec.Dir)
	}

	// One unmeasured warm-up first. The first build in a cold cache is a
	// compile, not a cache check, and averaging it in would inflate the floor
	// and over-subtract from every rung that follows.
	if b, err := implrun.Compile(spec); err == nil {
		b.Close()
	} else {
		return Floor{}, fmt.Errorf("the unmutated %s corner does not build, so its floor cannot be measured: %v\n%s",
			impl, err, b.Output)
	}

	f := Floor{Impl: impl, Samples: samples}
	var total, buildTotal float64
	for i := 0; i < samples; i++ {
		t0 := time.Now()
		b, err := implrun.Compile(spec)
		if err != nil {
			b.Close()
			return Floor{}, fmt.Errorf("build during floor sample %d: %v", i, err)
		}
		buildMS := msSince(t0)
		h, err := b.Start()
		if err != nil {
			b.Close()
			return Floor{}, fmt.Errorf("launch during floor sample %d: %v", i, err)
		}
		ms := msSince(t0)
		h.Stop()
		b.Close()

		total += ms
		buildTotal += buildMS
		if i == 0 || ms < f.MinMS {
			f.MinMS = ms
		}
		if ms > f.MaxMS {
			f.MaxMS = ms
		}
	}
	f.LaunchMS = total / float64(samples)
	f.BuildMS = buildTotal / float64(samples)
	return f, nil
}

func msSince(t time.Time) float64 {
	return float64(time.Since(t).Microseconds()) / 1000
}
