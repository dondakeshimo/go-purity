package purity

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/callgraph"
	"golang.org/x/tools/go/ssa"
)

type callKind uint8

const (
	kindCall callKind = iota
	kindDefer
	kindGo
)

// callSite records a call to an in-program function, whose verdict is resolved
// during the fixed point. External and builtin calls are classified
// immediately as local reasons instead.
type callSite struct {
	kind    callKind
	callees []*ssa.Function
	pos     token.Position
}

type funcInfo struct {
	fn           *ssa.Function
	localReasons []Reason   //exhaustruct:optional
	callSites    []callSite //exhaustruct:optional
}

func (fi *funcInfo) addReason(r Reason) {
	fi.localReasons = append(fi.localReasons, r)
}

// analyzeFunc walks a function body and collects local reasons and call sites.
// It enqueues the in-program callees it discovers.
func (a *Analyzer) analyzeFunc(fn *ssa.Function, cg *callgraph.Graph, prog *ssa.Program, enqueue func(*ssa.Function)) *funcInfo {
	si := newSharedInfo(fn)
	info := &funcInfo{fn: fn}
	for _, blk := range fn.Blocks {
		for _, instr := range blk.Instrs {
			a.instructionReasons(instr, si, cg, prog, info, enqueue)
		}
	}
	return info
}

func (a *Analyzer) instructionReasons(instr ssa.Instruction, si *sharedInfo, cg *callgraph.Graph, prog *ssa.Program, info *funcInfo, enqueue func(*ssa.Function)) {
	pos := prog.Fset.Position(instr.Pos())

	// Global reads: any value-use of a package-level variable is a read of
	// mutable global state and violates referential transparency.
	for idx, op := range instr.Operands(nil) {
		if op == nil {
			continue
		}
		if isAddrOperand(instr, idx) {
			continue
		}
		if rootOf(*op) == rootGlobal {
			info.addReason(Reason{Code: RReadGlobal, Pos: pos, Detail: (*op).Name()})
		}
	}

	switch ins := instr.(type) {
	case *ssa.Store:
		// A write to a function-local allocation is not a side effect: it is
		// only observable if the allocation escapes, and escape is always
		// accompanied by another impure operation (global/param store, channel
		// send, or a call). See docs/purity-spec/aliasing.md.
		switch rootOf(ins.Addr) {
		case rootGlobal:
			info.addReason(Reason{Code: RStoreGlobal, Pos: pos, Detail: ins.Addr.Name()})
		case rootParam:
			info.addReason(Reason{Code: RStoreParam, Pos: pos, Detail: ins.Addr.Name()})
		case rootLocal:
			// local write: pure
		case rootOther:
			info.addReason(Reason{Code: RStoreOther, Pos: pos, Detail: ins.Addr.Name(), Provisional: true})
		}

	case *ssa.MapUpdate:
		switch rootOf(ins.Map) {
		case rootLocal:
			// local map: pure
		case rootGlobal, rootParam:
			info.addReason(Reason{Code: RMapUpdate, Pos: pos, Detail: ins.Map.Name()})
		case rootOther:
			info.addReason(Reason{Code: RMapUpdate, Pos: pos, Detail: ins.Map.Name(), Provisional: true})
		}

	case *ssa.Send:
		info.addReason(Reason{Code: RSend, Pos: pos, Detail: ins.Chan.Name()})

	case *ssa.Select:
		info.addReason(Reason{Code: RSelect, Pos: pos})

	case *ssa.Panic:
		info.addReason(Reason{Code: RPanic, Pos: pos})

	case *ssa.Go:
		info.addReason(Reason{Code: RGo, Pos: pos})
		if cc := ins.Common(); cc != nil {
			if callee := cc.StaticCallee(); callee != nil {
				enqueue(callee)
			} else if _, ok := cc.Value.(*ssa.Builtin); !ok {
				for _, c := range dynamicCallees(cg, info.fn, ins) {
					enqueue(c)
				}
			}
		}

	case *ssa.UnOp:
		switch ins.Op {
		case token.MUL:
			if code, ok := a.readRefReason(ins.X, si); ok {
				info.addReason(Reason{Code: code, Pos: pos, Detail: ins.X.Name()})
			}
		case token.ARROW:
			info.addReason(Reason{Code: RRecv, Pos: pos, Detail: ins.X.Name()})
		}

	case *ssa.Field:
		if code, ok := a.readRefReason(ins.X, si); ok {
			info.addReason(Reason{Code: code, Pos: pos, Detail: ins.X.Name()})
		}

	case *ssa.Index:
		if code, ok := a.readRefReason(ins.X, si); ok {
			info.addReason(Reason{Code: code, Pos: pos, Detail: ins.X.Name()})
		}

	case *ssa.Lookup:
		if code, ok := a.readRefReason(ins.X, si); ok {
			info.addReason(Reason{Code: code, Pos: pos, Detail: ins.X.Name()})
		}

	case *ssa.Range:
		switch rangeKind(ins.X) {
		case rangeMap:
			info.addReason(Reason{Code: RMapRange, Pos: pos, Detail: ins.X.Name()})
		case rangeChan:
			info.addReason(Reason{Code: RRecv, Pos: pos, Detail: ins.X.Name()})
		case rangeOther:
			if code, ok := a.readRefReason(ins.X, si); ok {
				info.addReason(Reason{Code: code, Pos: pos, Detail: ins.X.Name()})
			}
		}

	case *ssa.Next:
		if kind := rangeKind(ins.Iter); kind == rangeChan {
			info.addReason(Reason{Code: RRecv, Pos: pos})
		} else if kind == rangeMap {
			info.addReason(Reason{Code: RMapRange, Pos: pos})
		} else if code, ok := a.readRefReason(ins.Iter, si); ok {
			info.addReason(Reason{Code: code, Pos: pos})
		}

	case *ssa.Convert:
		if isUnsafePointer(ins.Type()) || isUnsafePointer(ins.X.Type()) {
			info.addReason(Reason{Code: RUnsafe, Pos: pos})
		}

	case *ssa.ChangeType:
		if isUnsafePointer(ins.Type()) || isUnsafePointer(ins.X.Type()) {
			info.addReason(Reason{Code: RUnsafe, Pos: pos})
		}

	case *ssa.MultiConvert:
		if isUnsafePointer(ins.Type()) || isUnsafePointer(ins.X.Type()) {
			info.addReason(Reason{Code: RUnsafe, Pos: pos})
		}

	case *ssa.Call:
		a.handleCall(ins.Common(), ins, kindCall, si, cg, prog, info, enqueue)

	case *ssa.Defer:
		a.handleCall(ins.Common(), ins, kindDefer, si, cg, prog, info, enqueue)
	}
}

func (a *Analyzer) handleCall(cc *ssa.CallCommon, site ssa.CallInstruction, kind callKind, si *sharedInfo, cg *callgraph.Graph, prog *ssa.Program, info *funcInfo, enqueue func(*ssa.Function)) {
	pos := prog.Fset.Position(site.Pos())

	if b, ok := cc.Value.(*ssa.Builtin); ok {
		for _, r := range a.classifyBuiltin(b, cc.Args) {
			r.Pos = pos
			info.addReason(r)
		}
		return
	}

	if callee := cc.StaticCallee(); callee != nil {
		if len(callee.Blocks) == 0 {
			for _, r := range externalReasons(a.classifyExternal(callee)) {
				r.Pos = pos
				info.addReason(r)
			}
			return
		}
		enqueue(callee)
		info.callSites = append(info.callSites, callSite{kind: kind, callees: []*ssa.Function{callee}, pos: pos})
		return
	}

	// Dynamic dispatch.
	callees := dynamicCallees(cg, info.fn, site)
	if len(callees) == 0 {
		info.addReason(Reason{Code: RIndirectCall, Pos: pos})
		return
	}
	var inProgram []*ssa.Function
	for _, callee := range callees {
		if len(callee.Blocks) == 0 {
			for _, r := range externalReasons(a.classifyExternal(callee)) {
				r.Pos = pos
				info.addReason(r)
			}
			continue
		}
		enqueue(callee)
		inProgram = append(inProgram, callee)
	}
	if len(inProgram) > 0 {
		info.callSites = append(info.callSites, callSite{kind: kind, callees: inProgram, pos: pos})
	}
}

func externalReasons(v Verdict, reasons []Reason) []Reason {
	if v == Pure {
		return nil
	}
	return reasons
}

// readRefReason reports whether reading through v violates referential
// transparency under the configured read policy. Global reads are handled
// separately by the global-read operand scan.
func (a *Analyzer) readRefReason(v ssa.Value, si *sharedInfo) (ReasonCode, bool) {
	if a.cfg.ReadPolicy != ReadStrict {
		return 0, false
	}
	if !si.mayBeShared(v) {
		return 0, false
	}
	if rootOf(v) == rootParam {
		return RReadParam, true
	}
	return RReadShared, true
}

// isAddrOperand reports whether operand idx of instr is an address-only use
// (a write target or a taken address) rather than a value read.
func isAddrOperand(instr ssa.Instruction, idx int) bool {
	switch instr.(type) {
	case *ssa.Store, *ssa.MapUpdate, *ssa.FieldAddr, *ssa.IndexAddr:
		return idx == 0
	default:
		return false
	}
}

type rangeKindT uint8

const (
	rangeOther rangeKindT = iota
	rangeMap
	rangeChan
)

func rangeKind(x ssa.Value) rangeKindT {
	switch t := x.Type().Underlying().(type) {
	case *types.Map:
		return rangeMap
	case *types.Chan:
		return rangeChan
	case *types.Pointer:
		if _, ok := t.Elem().Underlying().(*types.Array); ok {
			return rangeOther
		}
	}
	return rangeOther
}
