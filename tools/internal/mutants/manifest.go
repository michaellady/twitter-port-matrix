// Package mutants is the semantic-mutant catalogue and the machinery that
// injects it into a copy of an implementation.
//
// The catalogue is DATA -- see tools/cmd/mutate/mutants.json -- and not Go
// control flow, for one reason: the matrix grows to four corners. Adding the
// Java and Kotlin rendering of "id allocation is off by one" has to be a
// manifest edit, or the injector becomes the thing that needs maintaining
// instead of the thing doing the measuring.
//
// Three invariants keep a mutant honest. Each exists because of a specific way
// this rig has already been wrong:
//
//  1. An anchor must match EXACTLY ONCE. During the Rust retarget three
//     str.replace patches silently no-oped because their anchors missed a
//     Gobra `// @ unfold` comment line, and R0 still climbed on other changes,
//     so the patches looked applied. A mutant that fails to apply is
//     indistinguishable from a rung that killed it -- it inflates every kill
//     rate in the table with a defect that was never injected.
//
//  2. A mutant tree is content-addressed. Same manifest plus same source gives
//     the same sha256, so a kill recorded today is reproducible later and
//     source drift under a mutant is visible as a changed address rather than
//     as a quietly different experiment.
//
//  3. A mutant must change OBSERVABLE behaviour. An equivalent mutant is
//     counted as killed by nothing and survived by every rung, which drags
//     every rung's measured kill rate down for a reason that has nothing to do
//     with the rung. `mutate probe` is the detector.
package mutants

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Text is a manifest string that may be written either as a plain JSON string
// or as an array of lines, joined with "\n".
//
// Anchors are multi-line source fragments carrying leading tabs. As one JSON
// string they are an unreviewable wall of \t and \n, and an anchor nobody can
// read against the file it targets is an anchor nobody checks -- which is how
// a silently no-oping patch survives review.
type Text struct{ s string }

// String returns the joined text.
func (t Text) String() string { return t.s }

// Empty reports whether the text is the empty string. A replacement may be
// empty (deleting a guard is a mutation); an anchor may not.
func (t Text) Empty() bool { return t.s == "" }

// UnmarshalJSON accepts either "a\nb" or ["a", "b"].
func (t *Text) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		t.s = s
		return nil
	}
	var lines []string
	if err := json.Unmarshal(b, &lines); err != nil {
		return fmt.Errorf("want a string or an array of lines, got %s", string(b))
	}
	t.s = strings.Join(lines, "\n")
	return nil
}

// MarshalJSON writes the joined form, which is what provenance records want.
func (t Text) MarshalJSON() ([]byte, error) { return json.Marshal(t.s) }

// An Edit is one anchored replacement inside one file of an implementation.
type Edit struct {
	File        string `json:"file"`
	Anchor      Text   `json:"anchor"`
	Replacement Text   `json:"replacement"`
}

// A Step is one request in a witness sequence.
type Step struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Body   string `json:"body,omitempty"`
}

// A Mutant is one semantic defect, rendered for one implementation.
//
// ID names the DEFECT and is shared across corners: `go/cursor-inclusive` and
// `rust/cursor-inclusive` are the same defect injected two ways. The kill
// table is per-port, so a defect that exists in only one corner cannot be
// compared across the matrix.
//
// Witness is an optional request sequence that is expected to tell the mutant
// apart from the original. Non-equivalence is an existence claim -- "there is
// some sequence on which they differ" -- so a witness is a proof of it, where
// random probing is only a search. It is written for the mutants no existing
// input source reaches: `id-burned-on-reject` is invisible to the R0 corpus
// (which rejects a duplicate handle at step 4 and never registers another
// user) and invisible to tracegen (which only ever registers fresh handles),
// so without a witness a live defect would be reported as equivalent.
type Mutant struct {
	ID          string `json:"id"`
	Impl        string `json:"impl"`
	Family      string `json:"family"`
	Description string `json:"description"`
	Witness     []Step `json:"witness,omitempty"`
	Edits       []Edit `json:"edits"`
}

// Key is the catalogue-wide unique name of a mutant.
func (m Mutant) Key() string { return m.Impl + "/" + m.ID }

// A Manifest is the whole catalogue.
type Manifest struct {
	Note     string            `json:"note"`
	Families map[string]string `json:"families"`
	Mutants  []Mutant          `json:"mutants"`
}

// Load reads and validates a manifest.
func Load(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(b))
	// A mistyped key would otherwise be dropped in silence, and a mutant with
	// a dropped "edits" key is a mutant that changes nothing.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

func (m *Manifest) validate() error {
	if len(m.Families) == 0 {
		return fmt.Errorf("no families declared")
	}
	if len(m.Mutants) == 0 {
		return fmt.Errorf("no mutants declared")
	}
	seen := map[string]bool{}
	for i, mu := range m.Mutants {
		where := fmt.Sprintf("mutant[%d]", i)
		if mu.ID == "" || mu.Impl == "" {
			return fmt.Errorf("%s: id and impl are required", where)
		}
		where = mu.Key()
		if seen[where] {
			return fmt.Errorf("%s: duplicate mutant key", where)
		}
		seen[where] = true
		if _, ok := m.Families[mu.Family]; !ok {
			return fmt.Errorf("%s: unknown family %q", where, mu.Family)
		}
		if mu.Description == "" {
			return fmt.Errorf("%s: description is required -- an unexplained mutant is unreadable in the kill table", where)
		}
		if len(mu.Edits) == 0 {
			return fmt.Errorf("%s: no edits, so it would inject nothing", where)
		}
		for j, w := range mu.Witness {
			if w.Method == "" || w.Path == "" {
				return fmt.Errorf("%s witness[%d]: method and path are required", where, j)
			}
		}
		for j, e := range mu.Edits {
			if e.File == "" {
				return fmt.Errorf("%s edit[%d]: file is required", where, j)
			}
			if e.Anchor.Empty() {
				return fmt.Errorf("%s edit[%d]: anchor is required", where, j)
			}
			if e.Anchor.String() == e.Replacement.String() {
				return fmt.Errorf("%s edit[%d]: replacement equals anchor, so the mutant is a no-op by construction", where, j)
			}
		}
	}
	return nil
}

// Get returns one mutant by corner and defect id.
func (m *Manifest) Get(impl, id string) (Mutant, error) {
	for _, mu := range m.Mutants {
		if mu.Impl == impl && mu.ID == id {
			return mu, nil
		}
	}
	var have []string
	for _, mu := range m.Mutants {
		if mu.Impl == impl {
			have = append(have, mu.ID)
		}
	}
	if len(have) == 0 {
		return Mutant{}, fmt.Errorf("no mutants for impl %q; catalogue has: %s",
			impl, strings.Join(m.Impls(), ", "))
	}
	sort.Strings(have)
	return Mutant{}, fmt.Errorf("unknown mutant %s/%s; that corner has:\n  %s",
		impl, id, strings.Join(have, "\n  "))
}

// Select returns the mutants matching the given filters; an empty filter
// matches everything. The result is in catalogue order, which is grouped by
// corner and family so a report reads in a stable order.
func (m *Manifest) Select(impl, id, family string) []Mutant {
	var out []Mutant
	for _, mu := range m.Mutants {
		if impl != "" && mu.Impl != impl {
			continue
		}
		if id != "" && mu.ID != id {
			continue
		}
		if family != "" && mu.Family != family {
			continue
		}
		out = append(out, mu)
	}
	return out
}

// Impls lists the corners the catalogue covers.
func (m *Manifest) Impls() []string {
	seen := map[string]bool{}
	var out []string
	for _, mu := range m.Mutants {
		if !seen[mu.Impl] {
			seen[mu.Impl] = true
			out = append(out, mu.Impl)
		}
	}
	sort.Strings(out)
	return out
}

// FamilyNames lists declared families in sorted order.
func (m *Manifest) FamilyNames() []string {
	var out []string
	for f := range m.Families {
		out = append(out, f)
	}
	sort.Strings(out)
	return out
}
