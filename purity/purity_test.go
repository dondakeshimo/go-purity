package purity

import (
	"testing"

	"github.com/dondakeshimo/go-purity/internal/testutil"
)

// check builds SSA for src, analyzes the function named target, and asserts the
// verdict and that each of wantCodes is present in the reasons.
func check(t *testing.T, src, target string, want Verdict, wantCodes ...ReasonCode) *Result {
	t.Helper()
	prog, fns := testutil.Build(t, src, target)
	a := New(Config{})
	rep, err := a.Analyze(prog, fns)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	res := rep.Results[fns[0]]
	if res == nil {
		t.Fatalf("no result for %q", target)
	}
	if res.Verdict != want {
		t.Fatalf("verdict = %v, want %v\nreasons: %s", res.Verdict, want, formatReasons(res))
	}
	for _, code := range wantCodes {
		if !hasCode(res, code) {
			t.Errorf("missing reason %v in:\n%s", code, formatReasons(res))
		}
	}
	return res
}

func checkStrict(t *testing.T, src, target string, want Verdict, wantCodes ...ReasonCode) *Result {
	t.Helper()
	prog, fns := testutil.Build(t, src, target)
	a := New(Config{ReadPolicy: ReadStrict})
	rep, err := a.Analyze(prog, fns)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	res := rep.Results[fns[0]]
	if res == nil {
		t.Fatalf("no result for %q", target)
	}
	if res.Verdict != want {
		t.Fatalf("verdict = %v, want %v\nreasons: %s", res.Verdict, want, formatReasons(res))
	}
	for _, code := range wantCodes {
		if !hasCode(res, code) {
			t.Errorf("missing reason %v in:\n%s", code, formatReasons(res))
		}
	}
	return res
}

func hasCode(res *Result, code ReasonCode) bool {
	for _, r := range res.Reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}

func formatReasons(res *Result) string {
	s := ""
	for _, r := range res.Reasons {
		if s != "" {
			s += ", "
		}
		s += r.Code.String()
		if r.Detail != "" {
			s += "(" + r.Detail + ")"
		}
	}
	return s
}

// --- Group 1: basic pure functions ---

func TestPureBasic(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"TC-P01 arithmetic", `package test
func f(x int) int { return x + 1 }`},
		{"TC-P02 locals", `package test
func f(x int) int { y := x * 2; return y - 1 }`},
		{"TC-P03 string concat", `package test
func f(s string) string { return s + "!" }`},
		{"TC-P04 array value read", `package test
func f(a [3]int) int { return a[0] + a[1] }`},
		{"TC-P05 struct value read", `package test
type P struct{ X, Y int }
func f(p P) int { return p.X + p.Y }`},
		{"TC-P06 local slice read", `package test
func f() int { s := []int{1, 2, 3}; return s[0] + s[1] }`},
		{"TC-P07 len + strings allowlist", `package test
import "strings"
func f(s string) int { return len(s) + strings.Count(s, "x") }`},
		{"TC-P08 pure recursion", `package test
func f(n int) int { if n <= 1 { return 1 }; return n * f(n-1) }`},
		{"TC-P10 pure closure", `package test
func f(x int) int { g := func(y int) int { return x + y }; return g(1) }`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check(t, c.src, "f", Pure)
		})
	}
}

func TestPureMutualRecursion(t *testing.T) {
	src := `package test
func isEven(n int) bool { if n == 0 { return true }; return isOdd(n - 1) }
func isOdd(n int) bool { if n == 0 { return false }; return isEven(n - 1) }
func f(n int) bool { return isEven(n) }`
	check(t, src, "f", Pure)
}

// --- Group 2: write side effects ---

func TestImpureWrites(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code ReasonCode
	}{
		{"TC-W01 global write", `package test
var g int
func f(x int) int { g = x; return x }`, RStoreGlobal},
		{"TC-W02 pointer param write", `package test
func f(p *int) int { *p = 1; return *p }`, RStoreParam},
		{"TC-W03 slice element write", `package test
func f(s []int) { s[0] = 1 }`, RStoreParam},
		{"TC-W05 map write", `package test
func f(m map[string]int) { m["a"] = 1 }`, RMapUpdate},
		{"TC-W06 append to param", `package test
func f(s []int) []int { return append(s, 1) }`, RAppend},
		{"TC-W07 copy", `package test
func f(dst, src []int) { copy(dst, src) }`, RCopy},
		{"TC-W08 delete", `package test
func f(m map[string]int) { delete(m, "a") }`, RDelete},
		{"TC-W09 clear", `package test
func f(m map[string]int) { clear(m) }`, RClear},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check(t, c.src, "f", Impure, c.code)
		})
	}
}

// --- Group 3: call propagation ---

func TestCallPropagation(t *testing.T) {
	t.Run("TC-C01 pure callee", func(t *testing.T) {
		src := `package test
func g(x int) int { return x + 1 }
func f(x int) int { return g(x) }`
		check(t, src, "f", Pure)
	})
	t.Run("TC-C02 impure callee", func(t *testing.T) {
		src := `package test
import "fmt"
func g(x int) int { fmt.Println(x); return x }
func f(x int) int { return g(x) }`
		check(t, src, "f", Impure, RCallsImpure)
	})
	t.Run("TC-C03 transitive", func(t *testing.T) {
		src := `package test
import "fmt"
func h() int { fmt.Println("x"); return 1 }
func g() int { return h() }
func f() int { return g() }`
		check(t, src, "f", Impure, RCallsImpure)
	})
	t.Run("TC-C04 direct IO", func(t *testing.T) {
		src := `package test
import "fmt"
func f() { fmt.Println("x") }`
		check(t, src, "f", Impure, RCallsImpure)
	})
}

// --- Group 4: globals ---

func TestGlobals(t *testing.T) {
	t.Run("TC-G01 read global var", func(t *testing.T) {
		src := `package test
var g = 1
func f() int { return g + 1 }`
		check(t, src, "f", Impure, RReadGlobal)
	})
	t.Run("TC-G02 read const", func(t *testing.T) {
		src := `package test
const C = 1
func f() int { return C + 1 }`
		check(t, src, "f", Pure)
	})
	t.Run("TC-G03 deref global pointer", func(t *testing.T) {
		src := `package test
var g *int
func f() int { return *g }`
		check(t, src, "f", Impure, RReadGlobal)
	})
}

// --- Group 5: control flow & concurrency ---

func TestControlFlow(t *testing.T) {
	cases := []struct {
		name string
		src  string
		code ReasonCode
	}{
		{"TC-F01 goroutine", `package test
func helper() {}
func f() { go helper() }`, RGo},
		{"TC-F02 defer impure", `package test
import "fmt"
func f() { defer fmt.Println("x") }`, RCallsImpure},
		{"TC-F03 panic", `package test
func f() { panic("x") }`, RPanic},
		{"TC-F04 channel send", `package test
func f(ch chan int) { ch <- 1 }`, RSend},
		{"TC-F05 channel recv", `package test
func f(ch chan int) int { return <-ch }`, RRecv},
		{"TC-F06 select", `package test
func f(ch1, ch2 chan int) int {
	select {
	case <-ch1:
		return 1
	case <-ch2:
		return 2
	}
}`, RSelect},
		{"TC-F07 map range", `package test
func f(m map[string]int) int { s := 0; for k := range m { s += len(k) }; return s }`, RMapRange},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check(t, c.src, "f", Impure, c.code)
		})
	}

	t.Run("TC-F02b defer pure", func(t *testing.T) {
		src := `package test
func f() { defer func() { _ = 1 }() }`
		check(t, src, "f", Pure)
	})
	t.Run("TC-F08 slice range", func(t *testing.T) {
		src := `package test
func f(s []int) int { sum := 0; for _, v := range s { sum += v }; return sum }`
		check(t, src, "f", Pure)
	})
}

// --- Group 6: unsafe / reflect ---

func TestUnsafeReflect(t *testing.T) {
	t.Run("TC-U01 unsafe", func(t *testing.T) {
		src := `package test
import "unsafe"
func f(p *int) uintptr { return uintptr(unsafe.Pointer(p)) }`
		check(t, src, "f", Impure, RUnsafe)
	})
	t.Run("TC-U02 reflect", func(t *testing.T) {
		src := `package test
import "reflect"
func f(x int) int { return int(reflect.ValueOf(x).Int()) }`
		check(t, src, "f", Impure, RReflect)
	})
}

// --- Group 7: standard library ---

func TestStdlib(t *testing.T) {
	t.Run("TC-S01 strings allowlist", func(t *testing.T) {
		src := `package test
import "strings"
func f(s string) string { return strings.ToUpper(s) + strings.TrimSpace(s) }`
		check(t, src, "f", Pure)
	})
	t.Run("TC-S02 math allowlist", func(t *testing.T) {
		src := `package test
import "math"
func f(x float64) float64 { return math.Sqrt(x) + math.Max(x, 1) }`
		check(t, src, "f", Pure)
	})
	t.Run("TC-S03 strconv allowlist", func(t *testing.T) {
		src := `package test
import "strconv"
func f(x int) string { return strconv.Itoa(x) }`
		check(t, src, "f", Pure)
	})
	t.Run("TC-S04 fmt denylist", func(t *testing.T) {
		src := `package test
import "fmt"
func f() { fmt.Println("x") }`
		check(t, src, "f", Impure, RCallsImpure)
	})
	t.Run("TC-S04b fmt.Sprintf denylist", func(t *testing.T) {
		src := `package test
import "fmt"
func f(x int) string { return fmt.Sprintf("%d", x) }`
		check(t, src, "f", Impure, RCallsImpure)
	})
	t.Run("TC-S05 time.Now denylist", func(t *testing.T) {
		src := `package test
import "time"
func f() time.Time { return time.Now() }`
		check(t, src, "f", Impure, RCallsImpure)
	})
	t.Run("TC-S06 rand denylist", func(t *testing.T) {
		src := `package test
import "math/rand"
func f() int { return rand.Int() }`
		check(t, src, "f", Impure, RCallsImpure)
	})
	t.Run("TC-S07 os.Getenv denylist", func(t *testing.T) {
		src := `package test
import "os"
func f() string { return os.Getenv("X") }`
		check(t, src, "f", Impure, RCallsImpure)
	})
	t.Run("TC-S08 sync mutex denylist", func(t *testing.T) {
		src := `package test
import "sync"
func f() { var mu sync.Mutex; mu.Lock() }`
		check(t, src, "f", Impure, RCallsImpure)
	})
	t.Run("TC-S09 unmodeled external", func(t *testing.T) {
		src := `package test
import "hash/adler32"
func f(b []byte) uint32 { return adler32.Checksum(b) }`
		check(t, src, "f", Unknown, RExternalUnmodeled)
	})
}

// --- Group 8: recursion ---

func TestRecursion(t *testing.T) {
	t.Run("TC-R02 impure mutual recursion", func(t *testing.T) {
		src := `package test
import "fmt"
func isEven(n int) bool { if n == 0 { fmt.Println("x"); return true }; return isOdd(n - 1) }
func isOdd(n int) bool { if n == 0 { return false }; return isEven(n - 1) }
func f(n int) bool { return isEven(n) }`
		check(t, src, "f", Impure, RCallsImpure)
	})
	t.Run("TC-R03 non-terminating pure recursion", func(t *testing.T) {
		src := `package test
func f(n int) int { return f(n) }`
		check(t, src, "f", Pure)
	})
}

// --- Group 9: dynamic dispatch ---

func TestDynamicDispatch(t *testing.T) {
	t.Run("TC-D01a interface all pure", func(t *testing.T) {
		src := `package test
type I interface{ M() int }
type T struct{}
func (T) M() int { return 1 }
func f(i I) int { return i.M() }`
		check(t, src, "f", Pure)
	})
	t.Run("TC-D01b interface impure impl", func(t *testing.T) {
		src := `package test
import "fmt"
type I interface{ M() int }
type T struct{}
func (T) M() int { fmt.Println("x"); return 1 }
func f(i I) int { return i.M() }`
		check(t, src, "f", Impure, RCallsImpure)
	})
	t.Run("TC-D01c interface unresolved", func(t *testing.T) {
		src := `package test
type I interface{ M() int }
func f(i I) int { return i.M() }`
		check(t, src, "f", Unknown)
	})
	t.Run("TC-D02 func value unresolved", func(t *testing.T) {
		src := `package test
func f(g func(int) int) int { return g(1) }`
		check(t, src, "f", Unknown)
	})
}

// --- Group 10: read policy ---

func TestReadPolicy(t *testing.T) {
	t.Run("TC-T01 slice sum weak pure", func(t *testing.T) {
		src := `package test
func f(s []int) int { sum := 0; for _, v := range s { sum += v }; return sum }`
		check(t, src, "f", Pure)
	})
	t.Run("TC-T01 strict impure", func(t *testing.T) {
		src := `package test
func f(s []int) int { sum := 0; for _, v := range s { sum += v }; return sum }`
		checkStrict(t, src, "f", Impure, RReadParam)
	})
	t.Run("TC-T02 deref weak pure", func(t *testing.T) {
		src := `package test
func f(p *int) int { return *p }`
		check(t, src, "f", Pure)
	})
	t.Run("TC-T02 deref strict impure", func(t *testing.T) {
		src := `package test
func f(p *int) int { return *p }`
		checkStrict(t, src, "f", Impure, RReadParam)
	})
	t.Run("TC-T03 global read both modes", func(t *testing.T) {
		src := `package test
var g int
func f() int { return g }`
		check(t, src, "f", Impure, RReadGlobal)
		checkStrict(t, src, "f", Impure, RReadGlobal)
	})
}

// --- Group 11: local writes are pure; escape analysis roadmap ---

func TestLocalWritesPure(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"TC-A01 local pointer write", `package test
func f() int { x := 0; p := &x; *p = 1; return x }`},
		{"TC-A02 local slice write", `package test
func f() []int { s := []int{1}; s[0] = 2; return s }`},
		{"TC-A03 local append", `package test
func f() []int { s := []int{}; s = append(s, 1); return s }`},
		{"TC-A04 local map update", `package test
func f() map[string]int { m := map[string]int{}; m["a"] = 1; return m }`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			check(t, c.src, "f", Pure)
		})
	}
}

func TestProvisional(t *testing.T) {
	// A write through a pointer of unknown origin (a call result) is impure
	// but provisional: a future escape analysis may prove it points to a
	// local allocation.
	src := `package test
func getPtr() *int { return new(int) }
func f() int { p := getPtr(); *p = 1; return *p }`
	res := check(t, src, "f", Impure, RStoreOther)
	if !res.Provisional {
		t.Errorf("expected Provisional=true, got false")
	}
}

// --- Group 12: no-body functions ---

func TestNoBody(t *testing.T) {
	t.Run("TC-E02 declaration without body", func(t *testing.T) {
		// A function declared but implemented elsewhere (e.g. assembly).
		// In SSA this appears as a function with no body.
		src := `package test
func f() int { return externalDecl() }
func externalDecl() int`
		check(t, src, "f", Unknown, RExternalUnmodeled)
	})
}
