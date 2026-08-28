package main

import (
	"fmt"
	"sort"
	"strings"
)

// Canary mutations.
//
// A gate that has never been shown to fail proves nothing. Until now R0's
// falsifiability was demonstrated by hand -- editing an implementation,
// running replay, reverting -- which meant the demonstration was not itself
// gated and could silently stop happening.
//
// These mutations corrupt the RESPONSE after it is received, simulating a
// wrong implementation without touching implementation source. That keeps the
// canary language-independent: the same four mutations will exercise the Java
// and Kotlin corners with no additional work.
//
// The check is inverted: with a canary active, R0 MUST fail. A canary run that
// passes is a harness bug, not a success, and is reported as a hard failure.
type mutation struct {
	name string
	desc string
	// apply returns the corrupted response, and whether it changed anything.
	apply func(status int, body string) (int, string, bool)
}

var mutations = map[string]mutation{
	"status": {
		name: "status",
		desc: "rewrite the first 2xx status to 500",
		apply: func(status int, body string) (int, string, bool) {
			if status >= 200 && status < 300 {
				return 500, body, true
			}
			return status, body, false
		},
	},
	"errcode": {
		name: "errcode",
		desc: "rewrite every error code to a wrong one",
		apply: func(status int, body string) (int, string, bool) {
			if code, ok := errCode(body); ok {
				return status, strings.Replace(body, `"`+code+`"`, `"wrong_code"`, 1), true
			}
			return status, body, false
		},
	},
	"order": {
		name: "order",
		desc: "reverse the tweets array, breaking F2 ordering only",
		apply: func(status int, body string) (int, string, bool) {
			const open = `"tweets":[`
			i := strings.Index(body, open)
			if i < 0 {
				return status, body, false
			}
			j := strings.Index(body[i:], "]")
			if j < 0 {
				return status, body, false
			}
			inner := body[i+len(open) : i+j]
			objs := splitObjects(inner)
			if len(objs) < 2 {
				return status, body, false
			}
			for l, r := 0, len(objs)-1; l < r; l, r = l+1, r-1 {
				objs[l], objs[r] = objs[r], objs[l]
			}
			return status, body[:i+len(open)] + strings.Join(objs, ",") + body[i+j:], true
		},
	},
	"cursor": {
		name: "cursor",
		desc: "replace a null next_cursor with a fabricated one",
		apply: func(status int, body string) (int, string, bool) {
			const nc = `"next_cursor":null`
			if !strings.Contains(body, nc) {
				return status, body, false
			}
			return status, strings.Replace(body, nc, `"next_cursor":999`, 1), true
		},
	},
}

// splitObjects splits a JSON array body at top-level commas.
func splitObjects(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func mutationNames() []string {
	var n []string
	for k := range mutations {
		n = append(n, k)
	}
	sort.Strings(n)
	return n
}

func describeMutations() string {
	var b strings.Builder
	for _, n := range mutationNames() {
		fmt.Fprintf(&b, "    %-8s %s\n", n, mutations[n].desc)
	}
	return b.String()
}
