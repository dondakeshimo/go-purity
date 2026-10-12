# go-purity

`go-purity` is a linter to check specified functions are pure.

Go is not oriented to functional programming (FP) , but it is valuable to use essense of FP.
Purity is one of the most important parts of FP, and it is easy to apply Go projects partially but it is hard to keep consistency of pure functions style.
`go-purity` supports developers (human and AI) to sustain purity style.

## Usage

TBU: This specification is prototype. We needs brashing up.

```go
//go:linter purity
func AwesomePureFunc(i1 int) int {
    return i1 + 1
}
```

## Development

- `mise` is required
