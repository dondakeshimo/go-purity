package purity

import (
	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/callgraph/cha"
	"golang.org/x/tools/go/ssa"
)

// buildCallGraph constructs the program's call graph using Class Hierarchy
// Analysis (CHA).
//
// We intentionally use CHA rather than VTA/RTA: purity analysis requires a
// sound (over-approximating) call graph, so that every possible callee of a
// dynamic call (interface method, function value) is examined. VTA is more
// precise but under-approximates for libraries without a main function: it
// prunes interface implementations whose types never flow to a parameter, which
// would wrongly let an impure implementation go undetected.
func buildCallGraph(prog *ssa.Program) *callgraph.Graph {
	return cha.CallGraph(prog)
}

// dynamicCallees returns the functions a dynamic call site may resolve to,
// according to the call graph.
func dynamicCallees(cg *callgraph.Graph, caller *ssa.Function, site ssa.CallInstruction) []*ssa.Function {
	node := cg.Nodes[caller]
	if node == nil {
		return nil
	}
	var out []*ssa.Function
	for _, e := range node.Out {
		if e.Site == site {
			out = append(out, e.Callee.Func)
		}
	}
	return out
}
