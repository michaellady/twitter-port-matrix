package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// harness launches an implementation and drives it over HTTP.
//
// It has no other channel into the implementation. There is deliberately no
// way for this harness to set a clock, seed state, or reach past the API --
// that capability is exactly the defect recorded in finding F001.
type harness struct {
	name string
	spec implSpec
	cmd  *exec.Cmd
	base string
	log  *os.File
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

func start(name string, spec implSpec) (*harness, error) {
	tmp, err := os.MkdirTemp("", "replay-"+name+"-")
	if err != nil {
		return nil, err
	}
	bin := filepath.Join(tmp, "server")
	repl := map[string]string{"bin": bin}

	if len(spec.Build) > 0 {
		argv := subst(spec.Build, repl)
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = spec.Dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("build failed: %v\n%s", err, out)
		}
	}

	port, err := freePort()
	if err != nil {
		return nil, err
	}
	argv := subst(spec.Run, repl)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = spec.Dir
	cmd.Env = append(os.Environ(), fmt.Sprintf("%s=127.0.0.1:%d", spec.AddrEnv, port))

	logf, err := os.Create(filepath.Join(tmp, "server.log"))
	if err != nil {
		return nil, err
	}
	cmd.Stdout, cmd.Stderr = logf, logf

	if err := cmd.Start(); err != nil {
		return nil, err
	}
	h := &harness{
		name: name, spec: spec, cmd: cmd,
		base: fmt.Sprintf("http://127.0.0.1:%d", port),
		log:  logf,
	}

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(h.base + spec.HealthPath)
		if err == nil {
			resp.Body.Close()
			return h, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	h.stop()
	b, _ := os.ReadFile(logf.Name())
	return nil, fmt.Errorf("%s did not become healthy within 20s\nserver log:\n%s", name, b)
}

func (h *harness) stop() {
	if h.cmd != nil && h.cmd.Process != nil {
		_ = h.cmd.Process.Kill()
		_, _ = h.cmd.Process.Wait()
	}
	if h.log != nil {
		h.log.Close()
	}
}
