---
name: maintain-documentation
description: Audit and fix all documentation surfaces for this repo — README.md, Go package doc comments, and example_test.go — against the actual source code. Use when documentation may be stale, after API changes, or when the user says "check docs", "fix readme", "update godoc", or "documentation is wrong".
version: "1.0.0"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash(go:*)
  - Bash(cat:*)
  - TodoWrite
  - WebFetch
---

# maintain-documentation

Audits every documentation surface in drain-go against the actual Go source and fixes any inaccuracies.

## Documentation surfaces

| Surface              | File(s)                                    | Rendered at                          |
|----------------------|--------------------------------------------|--------------------------------------|
| README               | `README.md`                                | GitHub repo page                     |
| Package godoc        | `drain.go`, `parser.go`, `tree.go`         | pkg.go.dev (package overview + API)  |
| Runnable example     | `example_test.go`                          | pkg.go.dev (Examples section)        |

## Audit procedure

Work through each surface in order. Track progress with TodoWrite.

### 1. Ground truth — read source

Always start here. Read the actual source before touching docs.

```
Read go.mod           → module path, Go version
Read drain.go         → exported types, constants, field names, defaults described in comments
Read parser.go        → New(), Train(), Match(), MatchWithStrategy(), Clusters() signatures and behaviour
Read tree.go          → internal algorithm (cross-check algorithm description prose in README)
```

### 2. README.md

Check every claim against source:

**Install section**
- `go get <module>` path must match the `module` line in `go.mod` exactly.

**Usage code block**
- Import path must match `go.mod` module path.
- All struct names, field names, method names must exist and be exported.
- Method signatures (return types, parameters) must match.
- `Result` field names used in `fmt.Printf` (`ClusterID`, `Change`, `Template`) must exist on `Result`.

**Configuration table**
- Every row must correspond to a real `Config` field name.
- Default values must match what `New()` applies when the field is zero.
- No fields from `Config` should be missing from the table.

**Algorithm behavior section**
- Prose about `Match` strategies (`FullSearchFallback`, `TreeOnly`, `FullSearchAlways`) must match the `SearchStrategy` constants and `MatchWithStrategy` logic.
- Claim about wildcard branch routing must match `ParametrizeNumericTokens` behaviour in `New()` and `addToTree()`.

### 3. Go doc comments (pkg.go.dev)

Scan exported symbols in `drain.go` and `parser.go` for comment accuracy:

- Package doc: citation and algorithm name correct.
- `Config` field comments: default values, constraints, and behaviour described correctly.
- `Mask` field comments: order of `Name`/`Pattern` matches struct layout.
- `Result` field comments (if any): consistent with what `result()` populates.
- `Parser` doc: concurrency guarantee ("safe for concurrent calls") must hold — verify `Train` uses `mu.Lock()` and `Match` uses `mu.RLock()`.
- `Match` doc: describes fallback strategy accurately.
- `Clusters()` doc: "ordered by ID" matches the `sort.Slice` in implementation.

### 4. example_test.go

- Import path must match `go.mod`.
- All API calls must match current signatures.
- `// Output:` block must match what `go test -run Example` actually produces. Run it:

```bash
cd <workspace> && go test -run Example -v ./...
```

If the output block is wrong, update it to match the actual output.

### 5. Run verification

After any edits:

```bash
cd <workspace>
go build ./...
go vet ./...
go test ./...
```

All must pass. Type errors or test failures are blocking — fix before presenting results.

## Reporting

After the audit, summarise findings in a table:

| Surface              | Issue                              | Action taken          |
|----------------------|------------------------------------|-----------------------|
| README install       | Wrong module path                  | Fixed                 |
| example_test.go      | No issues found                    | —                     |
| godoc (parser.go)    | No issues found                    | —                     |

If no issues are found in a surface, say so explicitly — don't skip it silently.
