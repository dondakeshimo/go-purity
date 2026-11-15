# go-purity

`go-purity` is a linter that checks that specified functions are pure.

Go is not oriented toward functional programming (FP), but it is valuable to use the essence of FP.
Purity is one of the most important aspects of FP. It is easy to apply to Go projects partially, but it is hard to keep the style of pure functions consistent.
`go-purity` helps developers (human and AI) sustain a pure style.

## Status

> Last updated: 2026-11-07

The purity analysis **core** is implemented. The frontend (CLI / analyzer) is not implemented yet.

| Component | Status |
|---|---|
| Core purity analysis (`purity` package) | ✅ Implemented (tests passing) |
| Purity spec (`docs/purity-spec/`) | ✅ Documented |
| Frontend (CLI / `go/analysis` analyzer) | 🚧 Not implemented |
| Escape analysis (phase 2) | 🔜 Roadmap |

### Core library

The core is a library that takes an `*ssa.Program` and target functions, and reports each function's verdict (`PURE` / `IMPURE` / `UNKNOWN`) with reasons.

```go
import "github.com/dondakeshimo/go-purity/purity"

a := purity.New(purity.Config{}) // ReadWeak by default
report, err := a.Analyze(prog, targets) // *ssa.Program, []*ssa.Function
for fn, res := range report.Results {
    fmt.Printf("%s: %s (%v)\n", fn.Name(), res.Verdict, res.Reasons)
}
```

See [`docs/purity-spec/`](./docs/purity-spec/) for the purity definition, the undecidable cases, and the allowlist/denylist.

## Usage

TBD: This specification is a prototype. It needs brushing up.

The frontend is not implemented yet; the `//go:linter purity` directive below is the intended usage.

```go
//go:linter purity
func AwesomePureFunc(i1 int) int {
    return i1 + 1
}
```

## Development

- `mise` is required. Run `mise install --locked` to install Go, golangci-lint,
  and the GitHub CLI.
- Tool requests in `mise.toml` use `latest`, with
  `minimum_release_age = "7d"` so releases are held back for a week.
  `mise.lock` pins the resolved versions; run `mise lock --bump` to update the
  lockfile within that policy.
- The `gh-actions-lock` extension is installed or updated by the `postinstall`
  hook. gh extensions are not covered by `mise.lock`, so the extension follows
  the latest release. Run `gh auth login` before installing.
- `gosec` is run via `go run`, so no separate installation is required.

Run the same checks as CI:

```sh
mise install --locked

go test -race -shuffle=on -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

gofmt -l .
go vet ./...
golangci-lint run ./...
go run github.com/securego/gosec/v2/cmd/gosec@v2.30.0 -fmt=text ./...
```

CI runs on every push to `main` and on every pull request via
[`.github/workflows/ci.yml`](./.github/workflows/ci.yml).
