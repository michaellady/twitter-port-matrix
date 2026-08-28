# F002 — Model and implementations disagree on the tweet-id origin

**Status:** confirmed
**Severity:** benign in itself. Recorded because of what it demonstrates.

## What was observed

`twitter.tla` initialises `nextTweetId = 0` and posts with
`[id |-> nextTweetId, ...]`, so in the model **the first tweet has id 0**.

Both implementations start at 1:

```go
// twitter_golang_formal_verification/internal/ids/ids.go:19
func New() (g *Generator) {
	g = &Generator{next: 1}
	...
}
```

with an explicit test locking the behaviour in — `ids_test.go:5`,
`TestNextStartsAtOne`. Both the legacy corpus and every generated corpus here
assert `"id":1` for the first tweet.

## Is it a bug?

No. F8 is about uniqueness and per-author monotonicity, and both survive a
uniform `+1` shift. Nothing is broken, and no user-visible behaviour is wrong.

## Why record it

Because nothing in the existing artefact set could have told you either way.

The model was checked by TLC. The code was checked by Verus and Gobra. The
corpus was checked against the code. **No check compared the model to the
code.** The two sides could disagree about any concrete value -- id origin,
clock origin, error precedence, sort tiebreak -- and every gate would still be
green, because no gate spanned the gap.

The id origin is the harmless instance of that class. F001 is the harmful one:
the same missing link let an unreachable `created_at` sit in the corpus, and
the harnesses were then written to make it reachable by writing to the clock
directly.

Two disagreements found in the first hour of building a link check, in a
project where four repositories, eighteen CI workflows, two deductive
verifiers and a model checker were all reporting green, is the actual result
here. The gates were not wrong about what they measured. Nothing measured the
join.

## How it is handled

`tlclink`'s abstraction function subtracts 1 from tweet ids when projecting
S_obs onto the model (`tools/cmd/tlclink/project.go`, decision D11). A
refinement mapping is not required to be the identity, so this is sound.

It is written down rather than absorbed silently, because the next reader
needs to know that S_obs ids and model ids are in different numbering, and
that the difference is intentional rather than a second undetected drift.
