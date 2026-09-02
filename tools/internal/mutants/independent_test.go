package mutants

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The two catalogues and where they live, relative to this package.
const (
	originalManifest    = "../../cmd/mutate/mutants.json"
	independentManifest = "../../cmd/mutate/mutants-independent.json"
)

// TestIndependentCatalogueDeclaresEverySource is the gate on GOAL.md queue
// item 4's second requirement.
//
// The whole point of the second catalogue is that its defects come from
// somewhere other than spec/s_obs/. A mutant with no recorded provenance is
// indistinguishable in the kill table from one somebody invented by reading
// the contract, which is the exact confusion the catalogue exists to resolve.
// So `source` is optional in the type (the original catalogue states its
// provenance once, in its note) and mandatory here.
func TestIndependentCatalogueDeclaresEverySource(t *testing.T) {
	man, err := Load(independentManifest)
	if err != nil {
		t.Fatalf("loading the independent catalogue: %v", err)
	}
	if len(man.Sources) == 0 {
		t.Fatal("the independent catalogue declares no sources at all")
	}
	for _, m := range man.Mutants {
		if strings.TrimSpace(m.Source) == "" {
			t.Errorf("%s: no source declared; a provenance claim that is not written down "+
				"cannot be checked against", m.Key())
		}
	}
	// Every declared source must carry an argument long enough to be one.
	for name, arg := range man.Sources {
		if !strings.Contains(arg, "INDEPENDENCE ARGUMENT") {
			t.Errorf("source %q states no INDEPENDENCE ARGUMENT", name)
		}
	}
}

// TestUnknownSourceIsRejected shows the gate above can fail, per GOAL.md
// standing rule 2. A validator that accepts any string in `source` would let a
// mutant claim a provenance nobody argued for.
func TestUnknownSourceIsRejected(t *testing.T) {
	body := map[string]any{
		"families": map[string]string{"f": "a family"},
		"sources":  map[string]string{"declared": "INDEPENDENCE ARGUMENT: because."},
		"mutants": []map[string]any{{
			"id": "m", "impl": "go", "family": "f", "description": "d",
			"source": "invented-on-the-spot",
			"edits":  []map[string]any{{"file": "x.go", "anchor": "a", "replacement": "b"}},
		}},
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "m.json")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Load(path)
	if err == nil {
		t.Fatal("an undeclared source loaded cleanly; the check cannot fail, so it proves nothing")
	}
	if !strings.Contains(err.Error(), "unknown source") {
		t.Fatalf("rejected for the wrong reason: %v", err)
	}
}

// TestCataloguesHaveDisjointIDs keeps the two kill tables readable.
//
// The original catalogue's ids are quoted as denominators throughout
// evidence/, and its ids are shared across corners on purpose so one defect
// can be compared port-to-port. An id appearing in both files would make
// "go/cursor-inclusive survived R2" ambiguous about which catalogue produced
// it, and the ambiguity would not be visible in any report.
func TestCataloguesHaveDisjointIDs(t *testing.T) {
	orig, err := Load(originalManifest)
	if err != nil {
		t.Fatalf("loading the original catalogue: %v", err)
	}
	indep, err := Load(independentManifest)
	if err != nil {
		t.Fatalf("loading the independent catalogue: %v", err)
	}
	seen := map[string]bool{}
	for _, m := range orig.Mutants {
		seen[m.ID] = true
	}
	for _, m := range indep.Mutants {
		if seen[m.ID] {
			t.Errorf("%s: id is already used by the original catalogue", m.ID)
		}
	}
}

// TestPortableIDsCoverEveryCorner holds the fourth requirement to its word.
//
// A defect rendered in only some corners cannot appear in a port-to-port row,
// and the catalogue's answer to that is to say so IN THE ID. Anything whose id
// does not name a corner (or corner family) must be rendered in all four; a
// language idiom that genuinely has no twin declares itself with a `go-` or
// `jvm-` prefix and is exempt.
func TestPortableIDsCoverEveryCorner(t *testing.T) {
	man, err := Load(independentManifest)
	if err != nil {
		t.Fatalf("loading the independent catalogue: %v", err)
	}
	corners := map[string][]string{}
	for _, m := range man.Mutants {
		corners[m.ID] = append(corners[m.ID], m.Impl)
	}
	for id, impls := range corners {
		if strings.HasPrefix(id, "go-") || strings.HasPrefix(id, "rust-") ||
			strings.HasPrefix(id, "jvm-") || strings.HasPrefix(id, "java-") ||
			strings.HasPrefix(id, "kotlin-") {
			continue // declares its own restriction
		}
		if len(impls) != 4 {
			t.Errorf("%s is rendered in %v only, but its id does not say so; either render it "+
				"in all four corners or rename it so the kill table cannot be read as a "+
				"port-to-port row", id, impls)
		}
	}
}
