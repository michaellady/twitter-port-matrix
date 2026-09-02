// Package implrun builds and launches an implementation and drives it over
// HTTP.
//
// It has no other channel into the implementation. There is deliberately no
// way to set a clock, seed state, or reach past the observable API -- that
// capability is exactly the defect recorded in finding F001, and a mutation
// rig that could reach inside would be measuring something other than what the
// rungs measure.
//
// Build and Start are separate calls, unlike the equivalent code in cmd/replay
// where one launch serves one run. A probe restarts a server once per trace so
// no state leaks between traces, and rebuilding a Rust corner per trace would
// cost more than the probe itself.
package implrun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A Spec is one implementation's launch contract, as declared in
// impls/registry.json.
type Spec struct {
	Dir        string            `json:"dir"`
	Language   string            `json:"language"`
	Build      []string          `json:"build"`
	Run        []string          `json:"run"`
	Env        map[string]string `json:"env"`
	HealthPath string            `json:"health_path"`
	Verifier   string            `json:"verifier"`
	// VerifyBuild compiles the corner's OBLIGATIONS against the tree, when it
	// has any. It is separate from Build because the two answer different
	// questions and F031 is what happens when only the first is asked: on the
	// Kotlin corner `mutate verify` reported 18/18 compile clean, and the R4
	// rung was then handed the same tree and could not build it, because the
	// mutant had changed a method signature that Obligations.kt still called
	// the old way. One cell of eighteen became a missing measurement and the
	// gate that exists to catch exactly that said ok.
	//
	// A corner with no obligations leaves it empty and nothing changes.
	VerifyBuild []string `json:"verify_build,omitempty"`
	Status      string   `json:"status"`
}

// A Registry is the whole implementation set.
type Registry struct {
	Note  string          `json:"note,omitempty"`
	Impls map[string]Spec `json:"impls"`
}

// LoadRegistry reads impls/registry.json (or a mutant registry emitted by
// `mutate apply`).
func LoadRegistry(path string) (Registry, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Registry{}, err
	}
	var r Registry
	if err := json.Unmarshal(b, &r); err != nil {
		return Registry{}, err
	}
	if len(r.Impls) == 0 {
		return Registry{}, fmt.Errorf("registry %s has no implementations", path)
	}
	return r, nil
}

// Get returns one implementation, naming the alternatives when it misses.
func (r Registry) Get(name string) (Spec, error) {
	s, ok := r.Impls[name]
	if ok {
		return s, nil
	}
	var names []string
	for k := range r.Impls {
		names = append(names, k)
	}
	sort.Strings(names)
	return Spec{}, fmt.Errorf("unknown implementation %q; registry has: %s", name, strings.Join(names, ", "))
}

// A Build is a compiled implementation, ready to launch.
type Build struct {
	Spec   Spec
	Bin    string // path substituted for {bin} in the run argv
	Output string // the compiler's own output, kept for reporting
	tmp    string
}

// Compile runs the implementation's declared build command.
//
// The compiler's output is returned on success as well as on failure. A
// mutant that compiles with warnings is still news, and the only way to know
// what a build did is to read what it said.
func Compile(spec Spec) (*Build, error) {
	tmp, err := os.MkdirTemp("", "implrun-")
	if err != nil {
		return nil, err
	}
	b := &Build{Spec: spec, Bin: filepath.Join(tmp, "server"), tmp: tmp}
	if len(spec.Build) == 0 {
		return b, nil
	}
	argv := subst(spec.Build, map[string]string{"bin": b.Bin})
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = spec.Dir
	out, err := cmd.CombinedOutput()
	b.Output = string(out)
	if err != nil {
		return b, fmt.Errorf("build failed in %s: %v", spec.Dir, err)
	}
	return b, nil
}

// CompileObligations builds the corner's obligation sources against the tree,
// which is what the proof rung will do before it can verify anything. A corner
// that declares no verify_build is a no-op and reports so.
//
// The obligations are compiled against the tree under test, never a pristine
// copy: on a mutant tree that is the whole point, since the mutated source is
// what the obligation calls.
func CompileObligations(spec Spec) (out string, ran bool, err error) {
	if len(spec.VerifyBuild) == 0 {
		return "", false, nil
	}
	tmp, err := os.MkdirTemp("", "implrun-obl-")
	if err != nil {
		return "", true, err
	}
	defer os.RemoveAll(tmp)
	argv := subst(spec.VerifyBuild, map[string]string{"bin": filepath.Join(tmp, "obligations")})
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = spec.Dir
	b, err := cmd.CombinedOutput()
	if err != nil {
		return string(b), true, fmt.Errorf("obligation build failed in %s: %v", spec.Dir, err)
	}
	return string(b), true, nil
}

// Close removes the build's scratch directory.
func (b *Build) Close() {
	if b.tmp != "" {
		_ = os.RemoveAll(b.tmp)
	}
}

// A Harness is a running implementation process.
type Harness struct {
	Base string
	cmd  *exec.Cmd
	log  *os.File
	cl   *http.Client
}

// Start launches a compiled implementation on a free port and waits for it to
// answer its health path.
func (b *Build) Start() (*Harness, error) {
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	argv := subst(b.Spec.Run, map[string]string{"bin": b.Bin})
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = b.Spec.Dir
	// Implementations differ in how they take a listen address: the Go server
	// reads ADDR="host:port", the Rust server reads PORT="port". The registry
	// declares the shape per implementation so adding the Java and Kotlin
	// corners needs no change here.
	portRepl := map[string]string{
		"port": strconv.Itoa(port),
		"addr": fmt.Sprintf("127.0.0.1:%d", port),
	}
	cmd.Env = os.Environ()
	for k, v := range b.Spec.Env {
		cmd.Env = append(cmd.Env, k+"="+subst([]string{v}, portRepl)[0])
	}
	logf, err := os.CreateTemp("", "implrun-log-")
	if err != nil {
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = logf, logf
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	h := &Harness{
		Base: fmt.Sprintf("http://127.0.0.1:%d", port),
		cmd:  cmd,
		log:  logf,
		cl:   &http.Client{Timeout: 10 * time.Second},
	}
	health := b.Spec.HealthPath
	if health == "" {
		health = "/healthz"
	}
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(h.Base + health)
		if err == nil {
			resp.Body.Close()
			return h, nil
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.Stop()
	logs, _ := os.ReadFile(logf.Name())
	return nil, fmt.Errorf("%s did not become healthy within 20s\nserver log:\n%s", b.Spec.Dir, logs)
}

// Stop kills the process and removes its log.
func (h *Harness) Stop() {
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
		_, _ = h.cmd.Process.Wait()
	}
	if h.log != nil {
		name := h.log.Name()
		h.log.Close()
		_ = os.Remove(name)
	}
}

// Do issues one request and returns the raw status and body. Nothing is parsed
// or normalised: D8 makes the exact bytes part of the contract, so a
// comparison has to see them.
func (h *Harness) Do(method, path, body string) (int, string, error) {
	var r io.Reader
	if body != "" {
		r = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, h.Base+path, r)
	if err != nil {
		return 0, "", err
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.cl.Do(req)
	if err != nil {
		return 0, "", err
	}
	raw, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp.StatusCode, string(raw), nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func subst(argv []string, repl map[string]string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		for k, v := range repl {
			a = strings.ReplaceAll(a, "{"+k+"}", v)
		}
		out[i] = a
	}
	return out
}
