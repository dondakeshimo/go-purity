package purity

import (
	"strings"

	"golang.org/x/tools/go/ssa"
)

// fullName returns a human-readable qualified name for an SSA function.
// Methods are not uniquely qualified in this minimal form; the caller relies
// on package-level allowlist/denylist for those.
func fullName(fn *ssa.Function) string {
	if obj := fn.Object(); obj != nil {
		if p := obj.Pkg(); p != nil {
			return p.Path() + "." + obj.Name()
		}
		return obj.Name()
	}
	pkg := ""
	if fn.Pkg != nil && fn.Pkg.Pkg != nil {
		pkg = fn.Pkg.Pkg.Path()
	}
	return pkg + "." + fn.Name()
}

// classifyExternal classifies a function with no SSA body (external package
// function, builtin-free declaration, etc.).
func (a *Analyzer) classifyExternal(fn *ssa.Function) (Verdict, []Reason) {
	pkgPath := ""
	if fn.Pkg != nil && fn.Pkg.Pkg != nil {
		pkgPath = fn.Pkg.Pkg.Path()
	}

	switch pkgPath {
	case "unsafe":
		return Impure, []Reason{{Code: RUnsafe, Detail: fullName(fn), Callee: fn}}
	case "reflect":
		return Impure, []Reason{{Code: RReflect, Detail: fullName(fn), Callee: fn}}
	case "C":
		return Impure, []Reason{{Code: RCgo, Detail: fullName(fn), Callee: fn}}
	}

	name := fullName(fn)
	if _, ok := a.allowExact[name]; ok {
		return Pure, nil
	}
	if _, ok := a.denyExact[name]; ok {
		return Impure, []Reason{{Code: RCallsImpure, Detail: name, Callee: fn}}
	}
	if _, ok := a.allowPkg[pkgPath]; ok {
		return Pure, nil
	}
	if _, ok := a.denyPkg[pkgPath]; ok {
		return Impure, []Reason{{Code: RCallsImpure, Detail: name, Callee: fn}}
	}
	return Unknown, []Reason{{Code: RExternalUnmodeled, Detail: name, Callee: fn}}
}

// classifyBuiltin classifies a call to a built-in function. The arguments are
// the call's arguments, used to decide whether a write target is local.
func (a *Analyzer) classifyBuiltin(b *ssa.Builtin, args []ssa.Value) []Reason {
	switch b.Name() {
	case "len", "cap", "min", "max", "complex", "real", "imag":
		return nil
	case "append", "copy", "clear", "delete":
		var code ReasonCode
		switch b.Name() {
		case "append":
			code = RAppend
		case "copy":
			code = RCopy
		case "clear":
			code = RClear
		case "delete":
			code = RDelete
		}
		// The write target is the first argument. A local target is a pure
		// local mutation; a reference-typed parameter or global is shared.
		kind := rootOther
		if len(args) > 0 {
			kind = rootOf(args[0])
		}
		if kind == rootLocal {
			return nil
		}
		return []Reason{{Code: code, Detail: b.Name(), Provisional: kind == rootOther}}
	case "close":
		return []Reason{{Code: RClose, Detail: b.Name()}}
	case "recover":
		return []Reason{{Code: RRecover, Detail: b.Name()}}
	case "print", "println":
		return []Reason{{Code: RPrint, Detail: b.Name()}}
	default:
		if strings.HasPrefix(b.Name(), "ssa:") {
			// Compiler-synthetic intrinsic (ssa:wrapnilchk, ssa:deferstack):
			// its observable effect is represented by other SSA constructs
			// (nil checks, defer instructions).
			return nil
		}
		return []Reason{{Code: RIndirectCall, Detail: "builtin " + b.Name()}}
	}
}

// initAllowlist seeds the built-in allowlist of known-pure functions.
func initAllowlist(a *Analyzer) {
	addPkg := func(pkg string) {
		a.allowPkg[pkg] = struct{}{}
	}
	add := func(pkg string, funcs ...string) {
		for _, f := range funcs {
			a.allowExact[pkg+"."+f] = struct{}{}
		}
	}

	// Packages whose exported functions are all pure.
	addPkg("math")
	addPkg("math/bits")
	addPkg("math/cmplx")
	addPkg("strconv")
	addPkg("unicode")
	addPkg("unicode/utf8")
	addPkg("unicode/utf16")
	addPkg("encoding/hex")
	addPkg("encoding/base32")
	addPkg("encoding/base64")
	addPkg("encoding/ascii85")
	addPkg("encoding/json")
	addPkg("crypto/md5")
	addPkg("crypto/sha1")
	addPkg("crypto/sha256")
	addPkg("crypto/sha512")

	// strings: pure functions (not the Builder type's methods).
	add("strings",
		"Clone", "Compare", "Contains", "ContainsAny", "ContainsRune",
		"Count", "Cut", "CutPrefix", "CutSuffix", "EqualFold", "Fields",
		"FieldsFunc", "HasPrefix", "HasSuffix", "Index", "IndexAny",
		"IndexByte", "IndexFunc", "IndexRune", "Join", "LastIndex",
		"LastIndexAny", "LastIndexByte", "LastIndexFunc", "Map", "Repeat",
		"Replace", "ReplaceAll", "Split", "SplitAfter", "SplitAfterN",
		"SplitN", "ToLower", "ToLowerSpecial", "ToTitle", "ToTitleSpecial",
		"ToUpper", "ToUpperSpecial", "Trim", "TrimFunc", "TrimLeft",
		"TrimLeftFunc", "TrimPrefix", "TrimRight", "TrimRightFunc",
		"TrimSpace", "TrimSuffix",
	)

	// bytes: pure functions (mirrors strings; not the Buffer type's methods).
	add("bytes",
		"Clone", "Compare", "Contains", "ContainsAny", "ContainsRune",
		"Count", "Cut", "CutPrefix", "CutSuffix", "Equal", "EqualFold",
		"Fields", "FieldsFunc", "HasPrefix", "HasSuffix", "Index",
		"IndexAny", "IndexByte", "IndexFunc", "IndexRune", "Join",
		"LastIndex", "LastIndexAny", "LastIndexByte", "LastIndexFunc",
		"Map", "Repeat", "Replace", "ReplaceAll", "Runes", "Split",
		"SplitAfter", "SplitAfterN", "SplitN", "ToLower", "ToLowerSpecial",
		"ToTitle", "ToTitleSpecial", "ToUpper", "ToUpperSpecial", "Trim",
		"TrimFunc", "TrimLeft", "TrimLeftFunc", "TrimPrefix", "TrimRight",
		"TrimRightFunc", "TrimSpace", "TrimSuffix",
	)

	// sort: read-only helpers (not the in-place sort functions).
	add("sort",
		"Search", "SearchFloat64s", "SearchInts", "SearchStrings",
		"IsSorted", "IntsAreSorted", "Float64sAreSorted", "StringsAreSorted",
	)

	// time: pure value constructors and parsers.
	add("time",
		"Date", "Unix", "UnixMicro", "UnixMilli", "Parse", "ParseDuration",
		"ParseInLocation", "Duration",
	)

	// regexp: compilation and pure matching.
	add("regexp",
		"Compile", "CompilePOSIX", "MustCompile", "MustCompilePOSIX",
		"Match", "MatchReader", "MatchString", "QuoteMeta",
	)

	// path: pure string manipulation.
	add("path", "Base", "Clean", "Dir", "Ext", "IsAbs", "Join", "Match", "Split")

	// path/filepath: pure string manipulation.
	add("path/filepath",
		"Base", "Clean", "Dir", "Ext", "FromSlash", "HasPrefix", "IsAbs",
		"Join", "Match", "Rel", "Split", "SplitList", "ToSlash", "VolumeName",
	)

	// net/url: pure parsing and escaping.
	add("net/url",
		"Parse", "ParseRequestURI", "PathEscape", "PathUnescape",
		"QueryEscape", "QueryUnescape", "User", "UserPassword",
	)
}

// initDenylist seeds the built-in denylist of impure/nondeterministic functions.
func initDenylist(a *Analyzer) {
	addPkg := func(pkg string) {
		a.denyPkg[pkg] = struct{}{}
	}
	add := func(pkg string, funcs ...string) {
		for _, f := range funcs {
			a.denyExact[pkg+"."+f] = struct{}{}
		}
	}

	// Packages whose functions are (broadly) impure. Package-level deny is
	// deliberately coarse: this is conservative and never reports a pure
	// function as impure-absent.
	addPkg("fmt")
	addPkg("log")
	addPkg("log/slog")
	addPkg("os")
	addPkg("io")
	addPkg("bufio")
	addPkg("net")
	addPkg("net/http")
	addPkg("net/http/httptest")
	addPkg("os/exec")
	addPkg("os/signal")
	addPkg("database/sql")
	addPkg("sync")
	addPkg("sync/atomic")
	addPkg("syscall")
	addPkg("runtime")
	addPkg("runtime/debug")
	addPkg("runtime/trace")
	addPkg("math/rand")
	addPkg("math/rand/v2")
	addPkg("crypto/rand")
	addPkg("context")
	addPkg("flag")
	addPkg("plugin")
	addPkg("testing")
	addPkg("testing/quick")

	// time: nondeterministic and blocking functions (the rest is pure).
	add("time",
		"Now", "Since", "Until", "Sleep", "After", "AfterFunc", "Tick",
		"NewTimer", "NewTicker",
	)
}
