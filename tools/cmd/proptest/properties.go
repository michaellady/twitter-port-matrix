package main

import (
	"fmt"
	"strconv"
)

// The relations. Each must hold whatever the correct answers are, so none of
// them consults S_obs.
var properties = []property{
	{
		name:  "follow-idempotent",
		about: "F3: following twice equals following once",
		check: func(c *client, _ int64, u []string) error {
			a, b := u[0], u[1]
			body := `{"from":"` + a + `","to":"` + b + `"}`
			r1 := c.do("POST", "/follow", body)
			t1, _ := c.timeline(a, "")
			r2 := c.do("POST", "/follow", body)
			t2, _ := c.timeline(a, "")
			if r1.status != r2.status || r1.body != r2.body {
				return fmt.Errorf("responses differ: %d %q then %d %q", r1.status, r1.body, r2.status, r2.body)
			}
			return sameTweets(t1, t2, "timeline changed on the second follow")
		},
	},
	{
		name:  "unfollow-idempotent",
		about: "F3: unfollowing twice equals unfollowing once",
		check: func(c *client, _ int64, u []string) error {
			a, b := u[0], u[1]
			body := `{"from":"` + a + `","to":"` + b + `"}`
			r1 := c.do("DELETE", "/follow", body)
			t1, _ := c.timeline(a, "")
			r2 := c.do("DELETE", "/follow", body)
			t2, _ := c.timeline(a, "")
			if r1.status != r2.status || r1.body != r2.body {
				return fmt.Errorf("responses differ: %d %q then %d %q", r1.status, r1.body, r2.status, r2.body)
			}
			return sameTweets(t1, t2, "timeline changed on the second unfollow")
		},
	},
	{
		name:  "follow-unfollow-restores",
		about: "follow then unfollow returns the timeline to its start",
		check: func(c *client, _ int64, u []string) error {
			a, b := u[0], u[1]
			body := `{"from":"` + a + `","to":"` + b + `"}`
			// Normalise: make sure we start from not-following.
			c.do("DELETE", "/follow", body)
			before, _ := c.timeline(a, "")
			c.do("POST", "/follow", body)
			c.do("DELETE", "/follow", body)
			after, _ := c.timeline(a, "")
			return sameTweets(before, after, "follow+unfollow was not a no-op")
		},
	},
	{
		name:  "pagination-partitions",
		about: "D10: pages partition the timeline, no loss, no fabrication",
		check: func(c *client, _ int64, u []string) error {
			a := u[0]
			full, r := c.timeline(a, "&limit=100")
			if r.status != 200 {
				return nil
			}
			seen := map[int64]bool{}
			var order []int64
			cursor := ""
			for pages := 0; ; pages++ {
				if pages > 200 {
					return fmt.Errorf("pagination did not terminate")
				}
				p, pr := c.timeline(a, "&limit=2"+cursor)
				if pr.status != 200 {
					return fmt.Errorf("page %d status %d", pages, pr.status)
				}
				for _, t := range p.Tweets {
					if seen[t.ID] {
						return fmt.Errorf("tweet %d appeared on two pages", t.ID)
					}
					seen[t.ID] = true
					order = append(order, t.ID)
				}
				if p.NextCursor == nil {
					break
				}
				cursor = "&cursor=" + strconv.FormatInt(*p.NextCursor, 10)
			}
			if len(seen) != len(full.Tweets) {
				return fmt.Errorf("paged set has %d tweets, single page has %d", len(seen), len(full.Tweets))
			}
			for i, t := range full.Tweets {
				if !seen[t.ID] {
					return fmt.Errorf("tweet %d missing from the paged set", t.ID)
				}
				if order[i] != t.ID {
					return fmt.Errorf("paged order differs at %d: %d vs %d", i, order[i], t.ID)
				}
			}
			return nil
		},
	},
	{
		name:  "timeline-ordered",
		about: "F2: strictly descending (created_at, id)",
		check: func(c *client, _ int64, u []string) error {
			for _, h := range u {
				p, r := c.timeline(h, "&limit=100")
				if r.status != 200 {
					continue
				}
				for i := 1; i < len(p.Tweets); i++ {
					prev, cur := p.Tweets[i-1], p.Tweets[i]
					if prev.CreatedAt < cur.CreatedAt ||
						(prev.CreatedAt == cur.CreatedAt && prev.ID <= cur.ID) {
						return fmt.Errorf("%s: out of order at %d: (%d,%d) then (%d,%d)",
							h, i, prev.CreatedAt, prev.ID, cur.CreatedAt, cur.ID)
					}
				}
			}
			return nil
		},
	},
	{
		name:  "visibility-follows-edges",
		about: "F1: a tweet is visible iff self-authored or followed",
		check: func(c *client, _ int64, u []string) error {
			a, b := u[0], u[1]
			body := `{"from":"` + a + `","to":"` + b + `"}`
			c.do("DELETE", "/follow", body)
			without, _ := c.timeline(a, "&limit=100")
			c.do("POST", "/follow", body)
			with, _ := c.timeline(a, "&limit=100")

			inWithout := map[int64]bool{}
			for _, t := range without.Tweets {
				inWithout[t.ID] = true
			}
			// Everything gained by following b must be authored by b.
			for _, t := range with.Tweets {
				if !inWithout[t.ID] && t.Author != b {
					return fmt.Errorf("following %s revealed a tweet by %s (id %d)", b, t.Author, t.ID)
				}
			}
			// Nothing may be lost by following.
			inWith := map[int64]bool{}
			for _, t := range with.Tweets {
				inWith[t.ID] = true
			}
			for _, t := range without.Tweets {
				if !inWith[t.ID] {
					return fmt.Errorf("following %s hid tweet %d", b, t.ID)
				}
			}
			return nil
		},
	},
	{
		name:  "post-prepends",
		about: "a new tweet by a followed author appears first",
		check: func(c *client, _ int64, u []string) error {
			a, b := u[0], u[1]
			c.do("POST", "/follow", `{"from":"`+a+`","to":"`+b+`"}`)
			before, _ := c.timeline(a, "&limit=100")
			c.do("POST", "/tick", "")
			r := c.do("POST", "/tweets", `{"author":"`+b+`","text":"probe"}`)
			if r.status != 201 {
				return fmt.Errorf("probe post failed: %d %s", r.status, r.body)
			}
			var posted tweet
			if err := jsonInto(r.body, &posted); err != nil {
				return err
			}
			after, _ := c.timeline(a, "&limit=100")
			if len(after.Tweets) != len(before.Tweets)+1 {
				return fmt.Errorf("timeline grew by %d, want 1", len(after.Tweets)-len(before.Tweets))
			}
			if after.Tweets[0].ID != posted.ID {
				return fmt.Errorf("new tweet %d is not first; first is %d", posted.ID, after.Tweets[0].ID)
			}
			return nil
		},
	},
	{
		name:  "clock-never-decreases",
		about: "F7: created_at is non-decreasing across ticks",
		check: func(c *client, _ int64, u []string) error {
			a := u[0]
			var last int64 = -1
			for i := 0; i < 4; i++ {
				r := c.do("POST", "/tweets", `{"author":"`+a+`","text":"probe"}`)
				if r.status == 201 {
					var t tweet
					if err := jsonInto(r.body, &t); err != nil {
						return err
					}
					if t.CreatedAt < last {
						return fmt.Errorf("created_at went backwards: %d then %d", last, t.CreatedAt)
					}
					last = t.CreatedAt
				}
				c.do("POST", "/tick", "")
			}
			return nil
		},
	},
	{
		name:  "reads-are-pure",
		about: "a GET never changes what the next GET returns",
		check: func(c *client, _ int64, u []string) error {
			a := u[0]
			p1, _ := c.timeline(a, "&limit=100")
			for i := 0; i < 3; i++ {
				c.timeline(a, "&limit=3")
				c.timeline(a, "&limit=1&cursor=2")
			}
			p2, _ := c.timeline(a, "&limit=100")
			return sameTweets(p1, p2, "reads mutated the timeline")
		},
	},
}

func sameTweets(a, b page, msg string) error {
	if len(a.Tweets) != len(b.Tweets) {
		return fmt.Errorf("%s: %d then %d tweets", msg, len(a.Tweets), len(b.Tweets))
	}
	for i := range a.Tweets {
		if a.Tweets[i] != b.Tweets[i] {
			return fmt.Errorf("%s: differ at %d: %+v vs %+v", msg, i, a.Tweets[i], b.Tweets[i])
		}
	}
	return nil
}
