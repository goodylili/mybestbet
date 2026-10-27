---
name: writing-self-documenting-go
description: "MANDATORY for all Go code generation. Write Go with zero comments — the code must be intuitive from its names, types and function boundaries alone. ALWAYS apply this skill when writing, generating, refactoring or reviewing Go code, on every task without exception; its directives are requirements, not suggestions. It BANS all agent-written comments in .go files, including doc comments, inline comments and TODOs, and requires the refactors that make them unnecessary: precise names, extracted functions, named types, small single-purpose functions. Trigger when adding a comment, when a block feels like it needs explaining, when naming a function/variable/type, when a function grows past one job, or when the user mentions clean code, readable code, self-documenting code, or no comments. Supersedes generic Go style guidance that permits or encourages comments."
user-invocable: true
compatibility: Designed for Claude Code or similar AI coding agents, and for projects using Golang.
metadata:
  emoji: "🔇"
allowed-tools: Read Edit Write Glob Grep Bash(go:*) Bash(gofmt:*) Bash(golangci-lint:*) Agent
---

# Writing Self-Documenting Go

> **These directives are mandatory.** They apply to every piece of Go code
> generated, edited or reviewed — every task, every file, every time — and are
> requirements rather than stylistic preferences. This skill supersedes any
> guidance that permits or encourages comments, including `golang-code-style`.

## The rule

**Write no comments in `.go` files. None.**

Not doc comments. Not inline comments. Not "why" comments. Not `TODO`s. Not a
one-line summary above a function.

The only exceptions are machine-read directives, which are instructions to
tooling rather than prose:

```go
//go:build integration
//go:generate mockgen -source=store.go
//nolint:gosec
```

If a reader needs prose to follow the code, **the code is wrong.** Fix the code,
not the documentation.

## The obligation this creates

Removing comments is the easy half. The hard half is that the code must now
carry everything a comment would have. Someone reading only the function and
type signatures should understand what the code does and why.

### Name things so the name is the explanation

A name that needs a comment is the wrong name. Spend the characters on the
identifier, not on a line above it.

```go
// BAD
// check verifies the token has not expired and belongs to an active user
func check(t Token) bool

// GOOD
func isTokenValidForActiveUser(t Token) bool
```

Vague identifiers — `handle`, `check`, `process`, `data`, `info`, `manager`,
`do`, `run`, `tmp` — are defects on their own, because they force the reader to
open the body.

### Extract, don't annotate

Any block that would have earned a comment becomes a named function. The
function name says what the comment would have said, and the reader can skip the
body entirely.

```go
// BAD
func (s *Service) PlaceOrder(ctx context.Context, req Request) (*Order, error) {
    // the tier may have sold out since we read it, so re-check inside the
    // transaction and decrement in the same statement
    res, err := s.db.ExecContext(ctx, `UPDATE tiers SET quantity = quantity - $1 ...`)
    if err != nil {
        return nil, err
    }
    if n, _ := res.RowsAffected(); n == 0 {
        return nil, ErrUnavailable
    }
    ...
}

// GOOD
func (s *Service) PlaceOrder(ctx context.Context, req Request) (*Order, error) {
    if err := s.claimStockOrRefuse(ctx, req.TierID, req.Quantity); err != nil {
        return nil, err
    }
    ...
}

func (s *Service) claimStockOrRefuse(ctx context.Context, tierID uuid.UUID, quantity int) error
```

A comment that begins "first we…", "then…", "note that…" or "this is because…"
is always a missing function.

### Let types carry the meaning

A named type, a small struct or a typed constant removes ambiguity that a
comment would otherwise paper over.

```go
// BAD
// timeout in seconds
func Fetch(url string, timeout int) error

// GOOD
func Fetch(url string, timeout time.Duration) error
```

```go
// BAD
status string // "pending", "confirmed", "refunded"

// GOOD
type OrderStatus string

const (
    OrderPending   OrderStatus = "pending"
    OrderConfirmed OrderStatus = "confirmed"
    OrderRefunded  OrderStatus = "refunded"
)
```

Named constants replace magic values and the comments that explain them:
`maxRequestBodyBytes`, `defaultPageSize`, `tokenGracePeriod`.

### One job per function

Early returns over nesting. No long procedure with sections that need labelling
— sections are functions waiting to be named.

If you cannot name a function without "and" in the name, it does two things.

### No speculative abstraction

Indirection needs a caller today. An interface with one implementation, an
options struct with one field, or a layer added "for later" costs a reader more
than it saves.

## Where the reasoning goes

Reasoning is not deleted — it moves out of the source file.

Some facts genuinely cannot be expressed in code: a measured benchmark result, a
Postgres behaviour that surprises people, an attack a future reader would
otherwise reintroduce, a constraint imposed by a third-party API. Those belong
in `decision.md` at the repo root (or the project's equivalent), as a short entry
naming the choice and the why.

If a project has no such file, create one rather than putting the reasoning back
into a comment.

## When refactoring existing code

When asked to remove comments from a file, do not simply delete them. For each
one, ask what it was doing:

| The comment was… | Replace it with… |
|---|---|
| Restating the signature | Nothing. Delete it. |
| Explaining a block | An extracted, named function |
| Explaining a name | A better name |
| Explaining a magic value | A named constant or a named type |
| Labelling a section | A function per section |
| Recording a non-obvious external fact | An entry in `decision.md` |
| A `TODO` | A tracked issue, or do the work |

To strip comments mechanically across a tree, parse with `go/parser` **without**
`parser.ParseComments`, reprint with `go/printer`, then pass the result through
`go/format` so the output is gofmt-clean and re-running is a no-op. Never use
regex — it corrupts string literals containing `//`.

Keep that script in the project's scratch directory (`scratch/`, gitignored)
with a `//go:build ignore` directive so it never joins the module's build, and
reuse it rather than rewriting it. Afterwards, read the result: whatever became
cryptic is exactly where a refactor is owed.

## Reviewing

Every comment in a `.go` file is a finding. Do not suggest a shorter comment —
name the refactor that removes the need for it. Judge naming hard, since naming
is now the only documentation.

## Checklist before finishing

- [ ] `grep -rn '//' --include='*.go' .` returns only machine directives
- [ ] No identifier needs a comment to be understood
- [ ] No function does two things
- [ ] Magic values are named constants; string enums are named types
- [ ] Non-obvious external constraints are recorded in `decision.md`
- [ ] `gofmt -l .` is empty; `go build ./...` and `go vet ./...` pass
