package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

// Resumability.
//
// A full sweep is 36 mutants times 4 rungs, and the rungs that matter most are
// the slow ones. A crash, a laptop lid, or an OOM two hours in must not send
// the run back to zero.
//
// Every completed measurement is appended to a journal and flushed, so at most
// the in-flight cell is lost. The journal is keyed by the mutant's CONTENT
// ADDRESS rather than by its name, which is the part that matters: three agents
// write this repository concurrently, so `impls/go` can change under a sweep.
// A name-keyed journal would resume by reusing yesterday's measurement of
// different code and there would be nothing in the output saying so. A
// hash-keyed one re-runs that mutant and leaves the stale cell unused.

// A Cell is one (mutant, rung) measurement -- the unit of the kill table and
// the unit of resumption.
type Cell struct {
	Mutant   string  `json:"mutant"`
	Impl     string  `json:"impl"`
	ID       string  `json:"id"`
	Family   string  `json:"family"`
	TreeHash string  `json:"tree_hash"`
	Rung     string  `json:"rung"`
	Outcome  string  `json:"outcome"`
	WallMS   float64 `json:"wall_ms"`
	Launches int     `json:"launches,omitempty"`
	ToolMS   float64 `json:"tool_ms,omitempty"`
	// ToolMeasured says whether ToolMS is a figure at all. A zero or negative
	// ToolMS is a real measurement -- the rung cost no more than the process
	// floor -- and must not read as "not measured".
	ToolMeasured bool         `json:"tool_measured,omitempty"`
	Verdict      string       `json:"verdict_line,omitempty"`
	Guard        *guardResult `json:"guard,omitempty"`
	Detail       string       `json:"detail,omitempty"`
	Error        string       `json:"error,omitempty"`
}

const (
	outcomeKilled     = "killed"
	outcomeSurvived   = "survived"
	outcomeUnreached  = "unreached"
	outcomeEquivalent = "equivalent"
	outcomeError      = "error"
	// outcomeCapped is a cell the rung cannot have: the corner has no
	// verifier for it (Rust has no Gobra rung; Kotlin has no deductive
	// verifier at all). It is written into the cell rather than left blank,
	// per GOAL.md queue item 3, and it is not a measurement: it counts in no
	// denominator and is never journalled.
	outcomeCapped = "capped"
	// outcomeUnclassified is what a survival is called when probing is off.
	// It is a separate word on purpose: "survived" is a claim about the rung,
	// and it must not be made on evidence that was never collected.
	outcomeUnclassified = "survived?"
)

type journalLine struct {
	Kind  string       `json:"kind"`
	Cell  *Cell        `json:"cell,omitempty"`
	Probe *ProbeRecord `json:"probe,omitempty"`
	Floor *Floor       `json:"floor,omitempty"`
}

type journal struct {
	path   string
	f      *os.File
	cells  map[string]Cell
	probes map[string]ProbeRecord
	floors map[string]Floor
}

func cellKey(mutant, tree, rung string) string { return mutant + "|" + tree + "|" + rung }
func probeKey(mutant, tree string) string      { return mutant + "|" + tree }

func openJournal(path string, resume bool) (*journal, error) {
	j := &journal{
		path:   path,
		cells:  map[string]Cell{},
		probes: map[string]ProbeRecord{},
		floors: map[string]Floor{},
	}
	if resume {
		if err := j.load(); err != nil {
			return nil, err
		}
	}
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if !resume {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return nil, fmt.Errorf("opening journal %s: %w", path, err)
	}
	j.f = f
	return j, nil
}

// load replays the journal. A malformed line is fatal rather than skipped: a
// half-written record means the previous run died mid-flush, and quietly
// dropping it would silently change which cells get re-run.
func (j *journal) load() error {
	f, err := os.Open(j.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		if len(sc.Bytes()) == 0 {
			continue
		}
		var line journalLine
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			return fmt.Errorf("%s:%d is malformed (%v); the previous run died mid-write. "+
				"Truncate the file or drop -resume rather than guessing what it held", j.path, n, err)
		}
		switch {
		case line.Cell != nil:
			c := *line.Cell
			// An errored cell is not a measurement. Resuming over one would
			// carry a hole in the table forward as if it were data.
			if c.Outcome == outcomeError {
				continue
			}
			j.cells[cellKey(c.Mutant, c.TreeHash, c.Rung)] = c
		case line.Probe != nil:
			p := *line.Probe
			j.probes[probeKey(p.Mutant, p.TreeHash)] = p
		case line.Floor != nil:
			j.floors[line.Floor.Impl] = *line.Floor
		}
	}
	return sc.Err()
}

func (j *journal) write(line journalLine) {
	b, err := json.Marshal(line)
	if err != nil {
		die("encoding journal line: %v", err)
	}
	if _, err := j.f.Write(append(b, '\n')); err != nil {
		die("writing journal: %v", err)
	}
	// Flushed per record. The whole point is that a crash costs one cell, and
	// a buffered journal would cost however much the buffer held.
	if err := j.f.Sync(); err != nil {
		die("flushing journal: %v", err)
	}
}

func (j *journal) appendCell(c Cell)         { j.write(journalLine{Kind: "cell", Cell: &c}) }
func (j *journal) appendProbe(p ProbeRecord) { j.write(journalLine{Kind: "probe", Probe: &p}) }
func (j *journal) appendFloor(f Floor)       { j.write(journalLine{Kind: "floor", Floor: &f}) }

func (j *journal) cell(mutant, tree, rung string) (Cell, bool) {
	c, ok := j.cells[cellKey(mutant, tree, rung)]
	return c, ok
}

func (j *journal) probe(mutant, tree string) (ProbeRecord, bool) {
	p, ok := j.probes[probeKey(mutant, tree)]
	return p, ok
}

func (j *journal) floor(impl string) (Floor, bool) {
	f, ok := j.floors[impl]
	return f, ok
}

func (j *journal) close() {
	if j != nil && j.f != nil {
		_ = j.f.Close()
	}
}
