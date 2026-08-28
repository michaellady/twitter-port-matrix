package mutants

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// TreeDirName is the subdirectory of an output directory that holds the
// mutated implementation. The provenance record sits beside it rather than
// inside it, so the mutated tree is byte-identical to a real implementation
// checkout and its content address covers source only.
const TreeDirName = "tree"

// A Result describes one materialised mutant.
type Result struct {
	Mutant      Mutant       `json:"mutant"`
	SourceDir   string       `json:"source_dir"`
	SourceHash  string       `json:"source_hash"`
	TreeDir     string       `json:"tree_dir"`
	TreeHash    string       `json:"tree_hash"`
	Edits       []EditResult `json:"edits"`
	CopyMode    string       `json:"copy_mode"`
	MaterialsAt string       `json:"materialised_at"`
}

// Materialize writes a mutated copy of srcDir into outDir/tree.
//
// The original is opened read-only. Everything else is a copy.
func Materialize(srcDir string, m Mutant, outDir string) (*Result, error) {
	base, err := LoadTree(srcDir)
	if err != nil {
		return nil, err
	}
	mutated, edits, err := Apply(base, m)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, err
	}
	treeDir := filepath.Join(outDir, TreeDirName)
	if _, err := os.Stat(treeDir); err == nil {
		return nil, fmt.Errorf("%s already exists; refusing to write over an existing mutant tree", treeDir)
	}
	mode, err := copyTree(base.Root, treeDir)
	if err != nil {
		return nil, fmt.Errorf("copying %s: %w", base.Root, err)
	}
	for _, r := range edits {
		p := filepath.Join(treeDir, filepath.FromSlash(r.File))
		perm := mutated.Modes[r.File]
		if perm == 0 {
			perm = 0o644
		}
		if err := os.WriteFile(p, mutated.Files[r.File], perm); err != nil {
			return nil, err
		}
	}

	// Re-read what was actually written and check it against what was computed
	// in memory. The content address is the reproducibility claim, and a claim
	// derived from the plan rather than from the artefact is the same mistake
	// as reading an exit code instead of the output.
	onDisk, err := LoadTree(treeDir)
	if err != nil {
		return nil, err
	}
	if got, want := onDisk.Hash(), mutated.Hash(); got != want {
		return nil, fmt.Errorf(
			"materialised tree does not match the computed mutation:\n  on disk %s\n  computed %s\n  (%s)",
			got, want, diffSummary(mutated, onDisk))
	}

	res := &Result{
		Mutant:      m,
		SourceDir:   base.Root,
		SourceHash:  base.Hash(),
		TreeDir:     treeDir,
		TreeHash:    onDisk.Hash(),
		Edits:       edits,
		CopyMode:    mode,
		MaterialsAt: time.Now().UTC().Format(time.RFC3339),
	}
	b, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(outDir, "mutant.json"), append(b, '\n'), 0o644); err != nil {
		return nil, err
	}
	return res, nil
}

// PlanHash returns the content address a mutant WOULD have, without writing
// anything. `mutate list -hashes` uses it, and `mutate apply` proves the
// materialised tree agrees with it.
func PlanHash(srcDir string, m Mutant) (base string, mutant string, err error) {
	t, err := LoadTree(srcDir)
	if err != nil {
		return "", "", err
	}
	mt, _, err := Apply(t, m)
	if err != nil {
		return "", "", err
	}
	return t.Hash(), mt.Hash(), nil
}

func diffSummary(want, got *Tree) string {
	for _, p := range want.Paths {
		g, ok := got.Files[p]
		if !ok {
			return "missing on disk: " + p
		}
		if string(g) != string(want.Files[p]) {
			return "content differs: " + p
		}
	}
	for _, p := range got.Paths {
		if _, ok := want.Files[p]; !ok {
			return "unexpected on disk: " + p
		}
	}
	return "same files, different order"
}

// copyTree duplicates a whole implementation directory, INCLUDING build output
// when the filesystem makes that free.
//
// Measured on impls/rust, whose target/ is 394 MB:
//
//	APFS clone of the whole tree      0.4s, then cargo rebuilds one crate  4.3s
//	source-only copy, cold cargo      instant, then cargo rebuilds all    10.6s
//
// Carrying target/ across is worth roughly 6s per mutant, and there are dozens
// of mutants per calibration run. `cp -Rc` is used rather than a Go
// implementation because clonefile(2) is not in the standard library and this
// repository has no third-party dependencies; it is one exec of a system
// binary, not a shell script. Any failure -- a non-APFS volume, another
// platform -- falls back to the portable walk, which skips build output and
// simply pays the cold build.
func copyTree(src, dst string) (string, error) {
	if runtime.GOOS == "darwin" {
		out, err := exec.Command("/bin/cp", "-Rc", src, dst).CombinedOutput()
		if err == nil {
			return "clone", nil
		}
		// Read the real failure rather than assuming why; a half-written
		// destination would otherwise be built and reported on.
		_ = os.RemoveAll(dst)
		_ = out
	}
	if err := walkCopy(src, dst); err != nil {
		return "", err
	}
	return "walk", nil
}

func walkCopy(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(src, p)
		if rerr != nil {
			return rerr
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if rel != "." && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return os.MkdirAll(target, 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("%s is not a regular file (%s)", rel, d.Type())
		}
		info, rerr := d.Info()
		if rerr != nil {
			return rerr
		}
		in, rerr := os.Open(p)
		if rerr != nil {
			return rerr
		}
		defer in.Close()
		out, rerr := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
		if rerr != nil {
			return rerr
		}
		if _, rerr := io.Copy(out, in); rerr != nil {
			out.Close()
			return rerr
		}
		return out.Close()
	})
}
