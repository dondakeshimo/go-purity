# go-purity

`go-purity` is a linter that checks that specified functions are pure.

Go is not oriented toward functional programming (FP), but it is valuable to use the essence of FP.
Purity is one of the most important aspects of FP. It is easy to apply to Go projects partially, but it is hard to keep the style of pure functions consistent.
`go-purity` helps developers (human and AI) sustain a pure style.

## Usage

TBD: This specification is a prototype. It needs brushing up.

```go
//go:linter purity
func AwesomePureFunc(i1 int) int {
    return i1 + 1
}
```

## Development

- `mise` is required
