package mutants

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// skipDirs are build output and VCS metadata rather than source.
//
// They are excluded from the tree, which means they are excluded from the
// content address: a mutant's identity must not depend on whether the corner
// happened to have been compiled, or the same defect would hash differently on
// two machines and the kill table would not be reproducible.
var skipDirs = map[string]bool{
	".git":   true,
	"target": true, // cargo output; 394 MB in impls/rust
}

// A Tree is an implementation's source, held in memory so a mutation can be
// applied, hashed, and inspected without ever writing to the original.
//
// Nothing in this package opens the original tree for writing. The rig judges
// implementations; an injector that can modify impls/ in place is one bad path
// join away from silently editing the thing it is supposed to be measuring.
type Tree struct {
	Root  string
	Paths []string // relative, slash-separated, sorted
	Files map[string][]byte
	Modes map[string]fs.FileMode
}

// LoadTree reads every source file under root.
func LoadTree(root string) (*Tree, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	t := &Tree{Root: abs, Files: map[string][]byte{}, Modes: map[string]fs.FileMode{}}
	err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, rerr := filepath.Rel(abs, p)
		if rerr != nil {
			return rerr
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel != "." && skipDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			// Refused rather than skipped: a skipped symlink is content that
			// silently leaves the hash, and the hash is the reproducibility
			// claim. If a corner ever needs one, decide it deliberately here.
			return fmt.Errorf("%s is not a regular file (%s); the tree hash covers regular files only", rel, d.Type())
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		info, rerr := d.Info()
		if rerr != nil {
			return rerr
		}
		t.Paths = append(t.Paths, rel)
		t.Files[rel] = b
		t.Modes[rel] = info.Mode().Perm()
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(t.Paths) == 0 {
		return nil, fmt.Errorf("%s contains no source files", root)
	}
	sort.Strings(t.Paths)
	return t, nil
}

// Clone returns a deep copy, so a mutation never aliases the loaded original.
func (t *Tree) Clone() *Tree {
	out := &Tree{
		Root:  t.Root,
		Paths: append([]string(nil), t.Paths...),
		Files: make(map[string][]byte, len(t.Files)),
		Modes: make(map[string]fs.FileMode, len(t.Modes)),
	}
	for p, b := range t.Files {
		c := make([]byte, len(b))
		copy(c, b)
		out.Files[p] = c
	}
	for p, m := range t.Modes {
		out.Modes[p] = m
	}
	return out
}

// Hash is the content address of the tree: sha256 over every source file in
// sorted path order.
//
// The path and the length are hashed alongside the bytes because concatenation
// alone is ambiguous -- a file "ab" holding "c" and a file "a" holding "bc"
// would otherwise collide, and a content address that can collide is not one.
func (t *Tree) Hash() string {
	h := sha256.New()
	var n [8]byte
	for _, p := range t.Paths {
		b := t.Files[p]
		binary.LittleEndian.PutUint64(n[:], uint64(len(p)))
		_, _ = h.Write(n[:])
		_, _ = h.Write([]byte(p))
		binary.LittleEndian.PutUint64(n[:], uint64(len(b)))
		_, _ = h.Write(n[:])
		_, _ = h.Write(b)
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// An EditResult records where an edit actually landed. It is reported rather
// than discarded so a reader can check the injection site against the source
// without re-deriving it.
type EditResult struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Offset   int    `json:"offset"`
	WasBytes int    `json:"was_bytes"`
	NowBytes int    `json:"now_bytes"`
}

// Apply returns a mutated copy of t, leaving t untouched.
//
// Every edit asserts that its anchor is present AND unique before replacing
// it. Both halves matter and for different reasons:
//
//   - absent means the source has drifted under the manifest. The mutant would
//     be a silent no-op, and a no-op mutant reads in the kill table exactly
//     like a defect every rung caught.
//   - non-unique means the manifest does not say WHICH site is meant. The
//     first match would be taken, the choice would be invisible, and the same
//     manifest would inject a different defect after an unrelated refactor.
//
// Edits are applied in order against the running result, so a later anchor is
// matched against the text an earlier edit produced.
func Apply(t *Tree, m Mutant) (*Tree, []EditResult, error) {
	out := t.Clone()
	results := make([]EditResult, 0, len(m.Edits))
	for i, e := range m.Edits {
		content, ok := out.Files[e.File]
		if !ok {
			return nil, nil, fmt.Errorf("%s edit[%d]: %s does not exist under %s",
				m.Key(), i, e.File, t.Root)
		}
		anchor := e.Anchor.String()
		s := string(content)
		n := strings.Count(s, anchor)
		switch {
		case n == 0:
			return nil, nil, fmt.Errorf(
				"%s edit[%d]: anchor not found in %s.\n"+
					"  The source has drifted under the manifest. Applying this mutant\n"+
					"  would inject nothing, and a mutant that injects nothing looks\n"+
					"  exactly like a defect every rung caught.\n"+
					"  anchor (first line): %s",
				m.Key(), i, e.File, firstLine(anchor))
		case n > 1:
			return nil, nil, fmt.Errorf(
				"%s edit[%d]: anchor matches %d sites in %s; an anchor must name one.\n"+
					"  anchor (first line): %s",
				m.Key(), i, n, e.File, firstLine(anchor))
		}
		off := strings.Index(s, anchor)
		repl := e.Replacement.String()
		out.Files[e.File] = []byte(s[:off] + repl + s[off+len(anchor):])
		results = append(results, EditResult{
			File:     e.File,
			Line:     1 + strings.Count(s[:off], "\n"),
			Offset:   off,
			WasBytes: len(anchor),
			NowBytes: len(repl),
		})
	}
	return out, results, nil
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i]) + " ..."
	}
	return strings.TrimSpace(s)
}
