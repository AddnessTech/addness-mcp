# MCP Server PR Review Guidelines

## 手順

1. リポジトリルートの `CLAUDE.md` を読み、アーキテクチャ・規約を把握する。
2. `gh pr diff $PR_NUMBER` で diff を取得する。
3. `CLAUDE.md` のルールと以下のチェック項目に基づいてレビューする。

## レビュー言語

日本語で記述する。

## 投稿ルール

- コード指摘は全て `mcp__github_inline_comment__create_inline_comment` でインラインコメントとして投稿してください
- 1つの指摘につき1つのコメントにしてください（まとめず個別に投稿）
- 全てのインラインコメントを投稿した後、`gh pr comment` でサマリーを1件だけ投稿してください
  - 指摘がない場合: 「指摘なし。LGTM」とだけ書いてください
  - 指摘がある場合: 指摘件数と、各指摘の1行要約をリストで書いてください

---

## セキュリティチェック

- ❌ API キー、トークン、パスワードのハードコード → 🚨指摘
- ❌ ユーザー入力が未検証のまま API リクエストに使用 → 🚨指摘
- ❌ HTTP クライアントにタイムアウト未設定 → 🚨指摘
- ❌ 機密情報が stderr/stdout に出力される → 🚨指摘

## MCP ツール設計チェック

- ❌ Tool description が不正確または不明瞭 → 🚨指摘
- ❌ `ids.Resolve()` 呼び出し漏れ（short ID → UUID 変換） → 🚨指摘
- ❌ `ids.Shorten()` 呼び出し漏れ（UUID → short ID 変換） → 🚨指摘
- ❌ エラーレスポンスが `errResult()` を使っていない → 🚨指摘
- ❌ API レスポンスの `unwrapData()` 漏れ → 🚨指摘

## コード品質チェック

- ❌ `http.DefaultClient` の使用（タイムアウトなし） → 🚨指摘
- ❌ レスポンスボディの Close 漏れ → 🚨指摘
- ❌ エラーハンドリングの欠落 → 🚨指摘

---

## 指摘不要

- フォーマット、import 順序（goimports/golangci-lint が管理）
- テストファイル (`*_test.go`)
- 設定ファイル、マークダウン、`.gitignore`、CI ワークフロー
