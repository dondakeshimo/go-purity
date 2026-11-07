package purity

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// rootKind classifies the origin of an address/value for reporting purposes.
type rootKind uint8

const (
	rootOther rootKind = iota
	rootLocal
	rootParam
	rootGlobal
)

// isReferenceType reports whether t is a reference type whose value aliases
// mutable state outside the function (pointer, slice, map, channel, interface,
// func, unsafe.Pointer). Strings are immutable and therefore not reference
// types for this purpose.
func isReferenceType(t types.Type) bool {
	switch u := t.Underlying().(type) {
	case *types.Pointer, *types.Slice, *types.Map, *types.Chan, *types.Interface, *types.Signature:
		return true
	case *types.Basic:
		return u.Kind() == types.UnsafePointer
	}
	return false
}

// isUnsafePointer reports whether t is the unsafe.Pointer type.
func isUnsafePointer(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.UnsafePointer
}

// rootOf walks an SSA value back to a coarse origin: a global, a parameter, a
// local allocation, or something else. It is used only for reporting and for
// the "provisional" flag; correctness of the verdict does not depend on it.
func rootOf(v ssa.Value) rootKind {
	switch x := v.(type) {
	case *ssa.Global:
		return rootGlobal
	case *ssa.Parameter:
		return rootParam
	case *ssa.FreeVar:
		return rootOther
	case *ssa.Alloc, *ssa.MakeSlice, *ssa.MakeMap, *ssa.MakeChan:
		return rootLocal
	case *ssa.FieldAddr:
		return rootOf(x.X)
	case *ssa.IndexAddr:
		return rootOf(x.X)
	case *ssa.Field:
		return rootOf(x.X)
	case *ssa.Index:
		return rootOf(x.X)
	case *ssa.Lookup:
		return rootOf(x.X)
	case *ssa.Slice:
		return rootOf(x.X)
	case *ssa.UnOp:
		if x.Op == token.MUL {
			return rootOf(x.X)
		}
		return rootOther
	case *ssa.Convert:
		return rootOf(x.X)
	case *ssa.ChangeType:
		return rootOf(x.X)
	case *ssa.Extract:
		return rootOf(x.Tuple)
	case *ssa.Range:
		return rootOf(x.X)
	case *ssa.Next:
		return rootOf(x.Iter)
	case *ssa.Phi:
		return rootOfPhi(x)
	default:
		return rootOther
	}
}

func rootOfPhi(p *ssa.Phi) rootKind {
	if len(p.Edges) == 0 {
		return rootOther
	}
	r := rootOf(p.Edges[0])
	for _, e := range p.Edges[1:] {
		r = mergeRoot(r, rootOf(e))
	}
	return r
}

// mergeRoot returns the "most shared" of two roots.
func mergeRoot(a, b rootKind) rootKind {
	if a == b {
		return a
	}
	if a == rootGlobal || b == rootGlobal {
		return rootGlobal
	}
	if a == rootParam || b == rootParam {
		return rootParam
	}
	if a == rootOther || b == rootOther {
		return rootOther
	}
	return rootLocal
}

// sharedInfo computes, per function, whether an SSA value may reference mutable
// state that is shared outside the function (globals, reference-typed
// parameters, call results, ...). This is a conservative intra-procedural
// dataflow: it errs on the side of reporting "shared".
type sharedInfo struct {
	fn       *ssa.Function
	memo     map[ssa.Value]bool
	visiting map[ssa.Value]bool
}

func newSharedInfo(fn *ssa.Function) *sharedInfo {
	return &sharedInfo{
		fn:       fn,
		memo:     map[ssa.Value]bool{},
		visiting: map[ssa.Value]bool{},
	}
}

// mayBeShared reports whether v may reference shared mutable state.
func (si *sharedInfo) mayBeShared(v ssa.Value) bool {
	if r, ok := si.memo[v]; ok {
		return r
	}
	if si.visiting[v] {
		return true // cycle in the value graph: be conservative
	}
	si.visiting[v] = true
	r := si.compute(v)
	delete(si.visiting, v)
	si.memo[v] = r
	return r
}

func (si *sharedInfo) compute(v ssa.Value) bool {
	switch x := v.(type) {
	case *ssa.Global:
		return true
	case *ssa.FreeVar:
		return true
	case *ssa.Parameter:
		return isReferenceType(x.Type())
	case *ssa.Call:
		return true // a call may return a reference into shared state
	case *ssa.Alloc, *ssa.MakeSlice, *ssa.MakeMap, *ssa.MakeChan:
		return false
	case *ssa.MakeClosure:
		for _, b := range x.Bindings {
			if si.mayBeShared(b) {
				return true
			}
		}
		return false
	case *ssa.Const, *ssa.Builtin, *ssa.Function:
		return false
	case *ssa.BinOp:
		return false
	case *ssa.Phi:
		for _, e := range x.Edges {
			if si.mayBeShared(e) {
				return true
			}
		}
		return false
	case *ssa.UnOp:
		switch x.Op {
		case token.MUL:
			return si.mayBeShared(x.X) || isReferenceType(x.Type())
		case token.ARROW:
			return isReferenceType(x.Type()) || si.mayBeShared(x.X)
		default:
			return false
		}
	case *ssa.Field:
		return si.mayBeShared(x.X) || isReferenceType(x.Type())
	case *ssa.Index:
		return si.mayBeShared(x.X) || isReferenceType(x.Type())
	case *ssa.Lookup:
		return si.mayBeShared(x.X) || isReferenceType(x.Type())
	case *ssa.Slice:
		return si.mayBeShared(x.X)
	case *ssa.Extract:
		return si.mayBeShared(x.Tuple) || isReferenceType(x.Type())
	case *ssa.TypeAssert:
		return si.mayBeShared(x.X) || isReferenceType(x.Type())
	case *ssa.MakeInterface:
		return si.mayBeShared(x.X)
	case *ssa.ChangeInterface:
		return si.mayBeShared(x.X)
	case *ssa.ChangeType:
		return si.mayBeShared(x.X)
	case *ssa.MultiConvert:
		return si.mayBeShared(x.X)
	case *ssa.Convert:
		return si.mayBeShared(x.X)
	case *ssa.FieldAddr:
		return si.mayBeShared(x.X)
	case *ssa.IndexAddr:
		return si.mayBeShared(x.X)
	case *ssa.SliceToArrayPointer:
		return si.mayBeShared(x.X)
	case *ssa.Range:
		return si.mayBeShared(x.X)
	case *ssa.Next:
		return si.mayBeShared(x.Iter)
	default:
		return true // unknown value kind: be conservative
	}
}
