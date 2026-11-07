# 純粋性仕様（purity spec）

`go-purity` の解析コアが採用する「純粋関数」の判定基準と、判定不能ケースの扱いを定める。

## 純粋関数の定義

純粋関数とは、次の 2 条件をともに満たす関数である。

1. **副作用がない**こと。
2. **参照透過**であること（結果が引数のみに依存する）。

解析コアは `golang.org/x/tools/go/ssa`（SSA/IR）上で、この 2 条件を「可能な限り十分に」検証する。

## 判定結果（3 値）

| Verdict | 意味 | デフォルト動作 |
|---|---|---|
| `PURE` | 純粋と証明できた | pass |
| `IMPURE` | 不純と証明できた（根拠 `Reason` 付き） | fail |
| `UNKNOWN` | 判定不能（原理的／実装限界） | fail（`IMPURE` と区別して報告） |

## 健全性スタンス

**健全性重視**（soundness-oriented）。`pure` と宣言した関数は「本当に純粋」であることを保証し、純粋と証明できないものは誤って `pure` にしない（保守的）。

- 判定不能（`UNKNOWN`）はデフォルトで fail として扱うが、`IMPURE` とは区別して報告する（allow 注釈などによる救済を想定）。
- 外部（標準ライブラリ等）の関数は allowlist/denylist 方式で扱い、どちらにも無い関数は `UNKNOWN`（→ fail）とする。

## 解析の範囲と制約

- 解析コアは `*ssa.Program` と対象関数（targets）を入力に取り、対象関数とそこから到達可能な関数を解析する。
- コールグラフは CHA（Class Hierarchy Analysis）を用いる。純粋性解析には「過大近似（sound）」なコールグラフが必要なため、より精緻だがライブラリ（`main` なし）で過小近似しうる VTA は採用しない。詳細は [aliasing.md](./aliasing.md) 参照。
- メモリ書き込みは、書き込み先の「根（root）」によって次のように分類する（詳細は [aliasing.md](./aliasing.md)）。

| 書き込み先 | 判定 |
|---|---|
| パッケージ変数（`Global`） | `IMPURE` |
| 参照型引数（ポインタ先・スライス要素・マップ要素） | `IMPURE` |
| 関数内ローカル割当（`Alloc`/`MakeSlice`/`MakeMap`） | `PURE` |
| 分類不能（呼び出し戻り値等） | `IMPURE`（`Provisional` フラグ付与） |

## 判定根拠（ReasonCode）

判定根拠は `purity.ReasonCode` として列挙する。主な種別は次の通り。

- 副作用: `RStoreGlobal` / `RStoreParam` / `RStoreOther` / `RMapUpdate` / `RSend` / `RGo` / `RSelect` / `RPanic` / `RAppend` / `RCopy` / `RClear` / `RDelete` / `RClose` / `RRecover` / `RPrint`
- 参照透過: `RReadGlobal` / `RReadParam` / `RReadShared` / `RMapRange` / `RRecv`
- 呼び出し: `RCallsImpure` / `RCallsUnknown` / `RIndirectCall` / `RExternalUnmodeled` / `RDefer`
- 特殊: `RUnsafe` / `RReflect` / `RCgo`

## 読み出しポリシー（ReadPolicy）

参照型引数（ポインタ先・スライス/マップ要素など）の読み出しは、参照透過性の解釈により扱いが分かれる。設定可能。

| 読み出し対象 | `ReadWeak`（既定） | `ReadStrict` |
|---|---|---|
| パッケージ変数 | `IMPURE` | `IMPURE` |
| 参照型引数経由 | `PURE` | `IMPURE` |
| ローカル・値型引数・`const`・`string` | `PURE` | `PURE` |

- 既定は `ReadWeak`（「副作用なし＋グローバル非依存」の実用解釈）。
- `ReadStrict` は深い値ベースの参照透過性を要求する厳格モード。
- この既定自体は判定不能ケース A.1〜A.3 に該当する仕様判断であり、最終決定は別途行う（[undecidable-cases.md](./undecidable-cases.md) 参照）。

## ドキュメント

- [undecidable-cases.md](./undecidable-cases.md) — 判定不能・判断が必要なケースの一覧
- [allowlist.md](./allowlist.md) — 標準ライブラリ／ビルトイン純粋性 DB
- [aliasing.md](./aliasing.md) — メモリ書き込み／エイリアス解析の現状とロードマップ
