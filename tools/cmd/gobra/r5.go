package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

// cmdR5 reports the state of every R5 clause by joining three things that are
// each recorded separately:
//
//	spec/refinement/obligations.json   what the clause claims, and whether a
//	                                   blocker was recorded against it
//	spec/refinement/clause-sites.json  which Gobra `ensures` carries it
//	the negation-canary results        whether Gobra can refute each of those
//
// A clause is reported VERIFIED only when every site carrying it verified AND
// every site was refutable when negated. "Gobra found no errors" on its own
// buys the first half only, and F013 is the reason the second half is not
// optional.
func cmdR5(args []string) error {
	fs := flag.NewFlagSet("r5", flag.ContinueOnError)
	obPath := fs.String("obligations", "spec/refinement/obligations.json", "")
	sitesPath := fs.String("sites", "spec/refinement/clause-sites.json", "")
	canPath := fs.String("canaries", "evidence/runs/gobra/negation-canaries.json", "")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var ob struct {
		Clauses []struct {
			Corner   string `json:"corner"`
			Layer    string `json:"layer"`
			Op       string `json:"op"`
			Clause   string `json:"clause"`
			Site     string `json:"site"`
			Status   string `json:"status"`
			Verifier string `json:"verifier"`
			Blocker  string `json:"blocker"`
		} `json:"clauses"`
		Blockers map[string]string `json:"blockers"`
	}
	if err := readJSON(*obPath, &ob); err != nil {
		return err
	}
	var sites struct {
		Clauses map[string]struct {
			EvidenceKind string `json:"evidence_kind"`
			Note         string `json:"note"`
			Sites        []struct {
				File   string `json:"file"`
				Member string `json:"member"`
				Text   string `json:"text"`
			} `json:"sites"`
		} `json:"clauses"`
	}
	if err := readJSON(*sitesPath, &sites); err != nil {
		return err
	}
	var canaries []canaryResult
	if err := readJSON(*canPath, &canaries); err != nil {
		return fmt.Errorf("%w (run `gobra canary -out %s` first)", err, *canPath)
	}
	// Keyed on (file, member, text): clause text alone is not unique, because
	// the same framing clause is repeated across many members.
	bySite := map[string]canaryResult{}
	for _, c := range canaries {
		bySite[c.Clause.File+"\x00"+c.Clause.Member+"\x00"+c.Clause.Text] = c
	}

	type row struct {
		N        int
		Corner   string
		Op       string
		Site     string
		Status   string
		Evidence string
		Summary  string
	}
	var rows []row
	counts := map[string]int{}
	for i, c := range ob.Clauses {
		n := i + 1
		s := sites.Clauses[strconv.Itoa(n)]
		r := row{N: n, Corner: c.Corner, Op: c.Op, Site: c.Site, Summary: c.Clause}

		switch {
		case len(s.Sites) == 0 && s.EvidenceKind == "fold":
			r.Status = "UNATTEMPTED"
			r.Evidence = "no postcondition to negate; discharged as a fold obligation, not canaried"
		case len(s.Sites) == 0:
			r.Status = "UNATTEMPTED"
			r.Evidence = "no Gobra site exists; recorded as " + c.Status
			if c.Blocker != "" {
				r.Evidence += " on " + c.Blocker + " — " + ob.Blockers[c.Blocker]
			}
		default:
			refuted, vac, undecided, missing := 0, 0, 0, 0
			var lines []string
			for _, st := range s.Sites {
				cr, ok := bySite[st.File+"\x00"+st.Member+"\x00"+st.Text]
				if !ok {
					missing++
					continue
				}
				switch cr.Verdict {
				case refutable:
					refuted++
					if len(cr.Errors) > 0 {
						lines = append(lines, cr.Errors[0])
					}
				case vacuous:
					vac++
				default:
					// TIMEOUT or ILL-FORMED. Neither answer was established.
					undecided++
				}
			}
			switch {
			case vac > 0:
				// One vacuous site is enough: the clause is not proved.
				r.Status = "VACUOUS"
				r.Evidence = fmt.Sprintf("%d of %d sites verified their own negation", vac, len(s.Sites))
			case missing > 0:
				r.Status = "UNATTEMPTED"
				r.Evidence = fmt.Sprintf("%d of %d sites not in the canary run", missing, len(s.Sites))
			case undecided > 0:
				// Not VERIFIED: the package is green, but a green package is
				// exactly the evidence F013 showed is compatible with an
				// empty obligation. Without a refuted negation there is no
				// per-clause evidence, so this clause is unaudited.
				r.Status = "UNAUDITED"
				r.Evidence = fmt.Sprintf("%d of %d sites: the solver did not decide the negation "+
					"within the budget, so reachability is unestablished", undecided, len(s.Sites))
			case refuted == len(s.Sites):
				r.Status = "VERIFIED"
				r.Evidence = firstLine(lines)
			default:
				r.Status = "UNAUDITED"
				r.Evidence = "no per-clause evidence"
			}
		}
		counts[r.Status]++
		rows = append(rows, r)
	}

	fmt.Println("R5 clause status — joined from Gobra's own output, not from obligations.json's")
	fmt.Println("recorded status. VERIFIED requires both a green package and a refutable negation.")
	fmt.Println()
	fmt.Printf("%-3s %-6s %-11s %-26s %-12s %s\n", "#", "CORNER", "OP", "SITE", "STATUS", "CLAUSE")
	for _, r := range rows {
		fmt.Printf("%-3d %-6s %-11s %-26s %-12s %s\n",
			r.N, r.Corner, trunc(r.Op, 11), trunc(r.Site, 26), r.Status, trunc(r.Summary, 62))
	}
	fmt.Println()
	keys := []string{"VERIFIED", "VACUOUS", "FAILED", "UNAUDITED", "UNATTEMPTED"}
	var parts []string
	for _, k := range keys {
		if counts[k] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[k], k))
		}
	}
	fmt.Printf("%d clauses: %s\n", len(rows), strings.Join(parts, ", "))

	fmt.Println("\nGobra's own line for each VERIFIED clause (from the negation canary that")
	fmt.Println("refuted it — this is the per-clause evidence a package-level green cannot give):")
	for _, r := range rows {
		if r.Status == "VERIFIED" {
			fmt.Printf("  %2d  %s\n      %s\n", r.N, r.Site, r.Evidence)
		}
	}
	if counts["UNAUDITED"] > 0 {
		fmt.Println("\nUNAUDITED — the package verifies with the clause present, but its negation")
		fmt.Println("was not decided, so nothing rules out the clause being vacuous:")
		for _, r := range rows {
			if r.Status == "UNAUDITED" {
				fmt.Printf("  %2d  %-26s %s\n      %s\n", r.N, r.Site, r.Evidence, trunc(r.Summary, 90))
			}
		}
	}
	if counts["UNATTEMPTED"] > 0 {
		fmt.Println("\nUNATTEMPTED, with the reason:")
		for _, r := range rows {
			if r.Status == "UNATTEMPTED" {
				fmt.Printf("  %2d  %-26s %s\n", r.N, r.Site, r.Evidence)
			}
		}
	}
	return nil
}

func firstLine(ls []string) string {
	if len(ls) == 0 {
		return "(refuted, but Gobra reported no location)"
	}
	sort.Strings(ls)
	return ls[0]
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
