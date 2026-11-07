// Package testutil provides helpers for building SSA programs from source in
// tests. It is for tests only; the purity core itself consumes *ssa.Program.
package testutil

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

// Build parses and type-checks src, builds SSA for it, and returns the program
// plus the top-level functions named by targets (in order).
func Build(t *testing.T, src string, targets ...string) (*ssa.Program, []*ssa.Function) {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	pkg := types.NewPackage("example.com/test", "test")
	tc := &types.Config{Importer: importer.Default()}
	spkg, _, err := ssautil.BuildPackage(tc, fset, pkg, []*ast.File{file}, ssa.SanityCheckFunctions|ssa.InstantiateGenerics)
	if err != nil {
		t.Fatalf("build ssa: %v", err)
	}

	fns := make([]*ssa.Function, 0, len(targets))
	for _, name := range targets {
		fn := spkg.Func(name)
		if fn == nil {
			t.Fatalf("function %q not found in SSA", name)
		}
		fns = append(fns, fn)
	}
	return spkg.Prog, fns
}
