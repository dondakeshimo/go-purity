# 標準ライブラリ／ビルトイン純粋性 DB

外部関数（SSA 本体を持たない関数）の純粋性は、allowlist（既知純粋）と denylist（非決定・I/O・並行・書込）で判定する。どちらにも無い関数は `UNKNOWN`（→ fail）。

> 本リストは**初期値**であり、今後拡充する。`purity/external.go` の `initAllowlist`/`initDenylist` が実体。

## 判定順序

1. `unsafe` パッケージ → `IMPURE`（`RUnsafe`）
2. `reflect` パッケージ → `IMPURE`（`RReflect`）
3. cgo（`import "C"`）→ `IMPURE`（`RCgo`）
4. allowlist（完全一致）→ `PURE`
5. denylist（完全一致）→ `IMPURE`
6. allowlist（パッケージ単位）→ `PURE`
7. denylist（パッケージ単位）→ `IMPURE`
8. それ以外 → `UNKNOWN`（`RExternalUnmodeled`）

## ビルトイン

| 分類 | ビルトイン |
|---|---|
| 純粋 | `len` `cap` `min` `max` `complex` `real` `imag` |
| 不純（書込系） | `append` `copy` `clear` `delete`（書き込み先がローカルなら純粋） |
| 不純 | `close` `panic` `recover` `print` `println` |
| 純粋（合成イントリンシック） | `ssa:wrapnilchk` `ssa:deferstack` |

## allowlist

### パッケージ単位（全関数が純粋）

`math` `math/bits` `math/cmplx` `strconv` `unicode` `unicode/utf8` `unicode/utf16` `encoding/hex` `encoding/base32` `encoding/base64` `encoding/ascii85` `encoding/json` `crypto/md5` `crypto/sha1` `crypto/sha256` `crypto/sha512`

### 関数単位（パッケージは混合のため）

- `strings.*` / `bytes.*`：純粋な文字列・バイト列操作（`Builder`/`Buffer` のメソッドは除く）
- `sort.*`：読み取り専用（`Search` `IsSorted` `*AreSorted` 系。インプレース整列 `Sort`/`Slice`/`Ints` 等は除く）
- `time.*`：値生成・解析（`Date` `Unix*` `Parse` `ParseDuration` 等。`Now`/`Sleep`/`After`/`Tick` 等は denylist）
- `regexp.*`：コンパイル・マッチ
- `path.*` / `path/filepath.*`：純粋なパス操作
- `net/url.*`：解析・エスケープ

## denylist

### パッケージ単位（大まかに不純）

`fmt` `log` `log/slog` `os` `io` `bufio` `net` `net/http` `net/http/httptest` `os/exec` `os/signal` `database/sql` `sync` `sync/atomic` `syscall` `runtime` `runtime/debug` `runtime/trace` `math/rand` `math/rand/v2` `crypto/rand` `context` `flag` `plugin` `testing` `testing/quick`

### 関数単位

- `time.Now` `time.Since` `time.Until` `time.Sleep` `time.After` `time.AfterFunc` `time.Tick` `time.NewTimer` `time.NewTicker`

## 既知の粗さ（要改善）

- `fmt.Sprintf`/`fmt.Sprint` は実際には副作用がないが、v1 では `fmt` パッケージ単位の denylist により不純扱い。将来、純粋な書式化関数を allowlist 化することを検討。
- `os.Getpid`/`os.Getppid` 等も同様にパッケージ単位 denylist で不純扱い。
