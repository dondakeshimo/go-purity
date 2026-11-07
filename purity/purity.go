// Package purity provides a static analysis that classifies Go functions as
// pure, impure, or unknown, using the SSA representation from
// golang.org/x/tools/go/ssa.
//
// A function is "pure" when it has no side effects and is referentially
// transparent: its result depends only on its arguments. This is a
// conservative (soundness-oriented) analysis: a function is reported as Pure
// only when purity can be proven. Anything that cannot be proven pure is
// reported as Impure (provably impure) or Unknown (unable to determine).
package purity

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// Verdict is the purity classification of a function.
type Verdict uint8

const (
	// Pure means the function is provably pure.
	Pure Verdict = iota
	// Impure means the function is provably impure.
	Impure
	// Unknown means purity could not be determined.
	Unknown
)

func (v Verdict) String() string {
	switch v {
	case Pure:
		return "pure"
	case Impure:
		return "impure"
	case Unknown:
		return "unknown"
	default:
		return "invalid"
	}
}

// ReadPolicy controls how reads of shared mutable state are treated for the
// referential-transparency half of the purity definition.
type ReadPolicy uint8

const (
	// ReadWeak (the default) allows reading through reference-typed parameters
	// (pointer pointees, slice/map elements). Only reads of package-level
	// variables violate referential transparency.
	ReadWeak ReadPolicy = iota
	// ReadStrict requires deep value semantics: any read of potentially shared
	// mutable state (pointer pointees, slice/map contents) violates
	// referential transparency.
	ReadStrict
)

// Config configures an Analyzer. The zero value is a valid configuration
// (ReadWeak, default allowlist/denylist).
type Config struct {
	// ReadPolicy selects the read policy. The zero value is ReadWeak.
	ReadPolicy ReadPolicy
}

// ReasonCode identifies why a function was classified as Impure or Unknown.
type ReasonCode uint8

const (
	RStoreGlobal       ReasonCode = iota // write to a package-level variable
	RStoreParam                          // write through a reference-typed parameter
	RStoreOther                          // write to an unclassifiable address (maybe local)
	RMapUpdate                           // map element write
	RSend                                // channel send
	RRecv                                // channel receive
	RGo                                  // goroutine
	RSelect                              // select statement
	RPanic                               // panic
	RDefer                               // defer of an impure function
	RMapRange                            // range over a map (nondeterministic order)
	RAppend                              // append
	RCopy                                // copy
	RClear                               // clear
	RDelete                              // delete
	RClose                               // close
	RRecover                             // recover
	RPrint                               // print / println
	RReadGlobal                          // read of a package-level variable
	RReadParam                           // read through a reference-typed parameter (strict only)
	RReadShared                          // read of other shared state (strict only)
	RCallsImpure                         // call to an impure function
	RCallsUnknown                        // call to an unknown function
	RIndirectCall                        // unresolved dynamic dispatch
	RExternalUnmodeled                   // external function not in allowlist/denylist
	RUnsafe                              // use of unsafe
	RReflect                             // use of reflect
	RCgo                                 // use of cgo
)

func (c ReasonCode) String() string {
	switch c {
	case RStoreGlobal:
		return "RStoreGlobal"
	case RStoreParam:
		return "RStoreParam"
	case RStoreOther:
		return "RStoreOther"
	case RMapUpdate:
		return "RMapUpdate"
	case RSend:
		return "RSend"
	case RRecv:
		return "RRecv"
	case RGo:
		return "RGo"
	case RSelect:
		return "RSelect"
	case RPanic:
		return "RPanic"
	case RDefer:
		return "RDefer"
	case RMapRange:
		return "RMapRange"
	case RAppend:
		return "RAppend"
	case RCopy:
		return "RCopy"
	case RClear:
		return "RClear"
	case RDelete:
		return "RDelete"
	case RClose:
		return "RClose"
	case RRecover:
		return "RRecover"
	case RPrint:
		return "RPrint"
	case RReadGlobal:
		return "RReadGlobal"
	case RReadParam:
		return "RReadParam"
	case RReadShared:
		return "RReadShared"
	case RCallsImpure:
		return "RCallsImpure"
	case RCallsUnknown:
		return "RCallsUnknown"
	case RIndirectCall:
		return "RIndirectCall"
	case RExternalUnmodeled:
		return "RExternalUnmodeled"
	case RUnsafe:
		return "RUnsafe"
	case RReflect:
		return "RReflect"
	case RCgo:
		return "RCgo"
	default:
		return "invalid"
	}
}

// Reason describes a single cause of impurity or uncertainty.
type Reason struct {
	// Code is the reason category.
	Code ReasonCode
	// Pos is the source position, if available.
	Pos token.Position
	// Detail is a human-readable note (variable name, callee name, ...).
	Detail string
	// Callee is the relevant function for call-related reasons.
	Callee *ssa.Function
	// Provisional reports that the impurity may disappear once a precise
	// escape analysis is implemented (e.g. writes to a local allocation).
	Provisional bool
}

// Result is the purity verdict for a single function.
type Result struct {
	Func    *ssa.Function
	Verdict Verdict
	Reasons []Reason
	// Provisional is true when the function is Impure solely due to
	// provisional reasons (writes that a future escape analysis may prove
	// local).
	Provisional bool
}

// Report is the result of an Analyze call.
type Report struct {
	// Results holds the verdict for each requested target function.
	Results map[*ssa.Function]*Result
	// All holds the verdict for every reachable in-program function that was
	// analyzed (for debugging).
	All map[*ssa.Function]*Result
}

// Analyzer performs the purity analysis.
type Analyzer struct {
	cfg        Config
	allowExact map[string]struct{}
	allowPkg   map[string]struct{}
	denyExact  map[string]struct{}
	denyPkg    map[string]struct{}
}

// New returns an Analyzer with the given configuration and the built-in
// allowlist/denylist.
func New(cfg Config) *Analyzer {
	a := &Analyzer{
		cfg:        cfg,
		allowExact: map[string]struct{}{},
		allowPkg:   map[string]struct{}{},
		denyExact:  map[string]struct{}{},
		denyPkg:    map[string]struct{}{},
	}
	initAllowlist(a)
	initDenylist(a)
	return a
}

// Analyze computes purity verdicts for targets and the functions reachable
// from them.
//
// prog must have its function bodies built (e.g. via Program.Build or
// ssautil.BuildPackage). targets are the entry functions to analyze.
func (a *Analyzer) Analyze(prog *ssa.Program, targets []*ssa.Function) (*Report, error) {
	cg := buildCallGraph(prog)

	// Discover reachable in-program functions and collect per-function info.
	infos := map[*ssa.Function]*funcInfo{}
	seen := map[*ssa.Function]bool{}
	queue := []*ssa.Function{}

	enqueue := func(fn *ssa.Function) {
		if fn == nil || seen[fn] {
			return
		}
		seen[fn] = true
		queue = append(queue, fn)
	}
	for _, t := range targets {
		enqueue(t)
	}

	for len(queue) > 0 {
		fn := queue[0]
		queue = queue[1:]
		if len(fn.Blocks) == 0 {
			continue // external function: classified as a leaf
		}
		if _, ok := infos[fn]; ok {
			continue
		}
		infos[fn] = a.analyzeFunc(fn, cg, prog, enqueue)
	}

	// Least fixed point over the call graph.
	verdicts := make(map[*ssa.Function]Verdict, len(infos))
	for fn := range infos {
		verdicts[fn] = Pure
	}
	for {
		changed := false
		for fn, info := range infos {
			v := Pure
			for _, r := range info.localReasons {
				v = maxVerdict(v, reasonVerdict(r))
			}
			for _, cs := range info.callSites {
				for _, callee := range cs.callees {
					v = maxVerdict(v, verdicts[callee])
				}
			}
			if v != verdicts[fn] {
				verdicts[fn] = v
				changed = true
			}
		}
		if !changed {
			break
		}
	}

	report := &Report{
		Results: map[*ssa.Function]*Result{},
		All:     map[*ssa.Function]*Result{},
	}
	for fn, info := range infos {
		report.All[fn] = a.resultFor(fn, info, verdicts)
	}
	for _, t := range targets {
		if res, ok := report.All[t]; ok {
			report.Results[t] = res
			continue
		}
		// External target (no SSA body).
		v, rs := a.classifyExternal(t)
		report.Results[t] = &Result{Func: t, Verdict: v, Reasons: rs}
	}
	return report, nil
}

// resultFor assembles the final Result for an in-program function.
func (a *Analyzer) resultFor(fn *ssa.Function, info *funcInfo, verdicts map[*ssa.Function]Verdict) *Result {
	verdict := verdicts[fn]
	reasons := append([]Reason(nil), info.localReasons...)
	for _, cs := range info.callSites {
		for _, callee := range cs.callees {
			cv := verdicts[callee]
			if cv == Pure {
				continue
			}
			code := RCallsImpure
			if cv == Unknown {
				code = RCallsUnknown
			}
			if cs.kind == kindDefer && cv == Impure {
				code = RDefer
			}
			reasons = append(reasons, Reason{
				Code:   code,
				Callee: callee,
				Detail: callee.Name(),
				Pos:    cs.pos,
			})
		}
	}

	provisional := false
	if verdict == Impure && len(reasons) > 0 {
		provisional = true
		for _, r := range reasons {
			if !r.Provisional {
				provisional = false
				break
			}
		}
	}
	return &Result{Func: fn, Verdict: verdict, Reasons: reasons, Provisional: provisional}
}

func reasonVerdict(r Reason) Verdict {
	switch r.Code {
	case RIndirectCall, RExternalUnmodeled, RCallsUnknown:
		return Unknown
	default:
		return Impure
	}
}

func maxVerdict(a, b Verdict) Verdict {
	if severity(a) >= severity(b) {
		return a
	}
	return b
}

func severity(v Verdict) int {
	switch v {
	case Pure:
		return 0
	case Unknown:
		return 1
	case Impure:
		return 2
	default:
		return 0
	}
}
