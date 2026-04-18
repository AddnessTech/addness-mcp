---
name: ship
description: Ship changes end-to-end. Local verify → PR create → review fix → CI watch → merge. Use when user says "ship", "マージして", "PRまでやって", or wants to land changes on main.
---

# Ship - 変更を main にランディングするまで自動化

ローカル検証 → PR作成 → レビュー対応 → CI通過 → マージ → タグ → ゴール記録 を一気通貫で実行する。

---

## Phase 1: ローカル検証

PR を作る前にローカルで全チェックを通す。**1つでも失敗したらその場で修正してから次へ進む。**

```bash
# 1. goimports（フォーマット修正も実行）
# goimports が PATH になければ: go install golang.org/x/tools/cmd/goimports@latest
$(go env GOPATH)/bin/goimports -w -local github.com/AddnessTech .

# 2. ビルド
go build .

# 3. リント（インストール済みの場合）
golangci-lint run .

# 4. テスト
go test . -race
```

### 失敗時の対応

- goimports diff がある → 自動修正済みなのでコミットに含める
- lint エラー → 該当コードを修正して再実行
- テスト失敗 → テストまたは実装を修正して再実行
- **全チェックが通るまで Phase 2 に進まない**

---

## Phase 2: PR作成

```bash
# 1. フィーチャーブランチにいることを確認（main なら作成）
BRANCH=$(git branch --show-current)
if [ "$BRANCH" = "main" ]; then
  echo "ERROR: main ブランチでは ship できません。フィーチャーブランチを作成してください。"
  exit 1
fi

# 2. 変更をコミット（未コミットがあれば）
git status

# 3. push
git push -u origin "$BRANCH"

# 4. PR作成
gh pr create --base main --title "..." --body "..."
```

### PR タイトル・ボディ規約

- タイトル: 70文字以内、`feat:` / `fix:` / `refactor:` / `chore:` プレフィックス
- ボディ: `## Summary` + `## Test plan` セクション必須

```
gh pr create --base main --title "fix(mcp): short ID cache persistence" --body "$(cat <<'EOF'
## Summary
- ShortIDCache をファイルに永続化し、MCP サーバー再起動時のキャッシュロスを防止

## Test plan
- [ ] ユニットテスト追加・通過
- [ ] ローカル lint / build 通過

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Phase 3: レビュー対応

PR 作成後、以下の GitHub Actions が起動する:

| Workflow | ファイル | トリガー |
|----------|----------|----------|
| **Claude PR Review** | `claude_pr_review.yml` | PR opened |
| **Lint Check** | `lint.yml` | PR to main |
| **Test** | `test.yml` | PR |
| **Migration Check** | `migration-check.yml` | PR to main |

### 3-1: Claude PR Review への対応（最優先）

PR 作成直後、**まずレビューを確認する。CI は待たない。**
レビュー指摘で修正 → push すると CI が再実行されるので、先に CI を待つのは無駄。

```bash
# PR のレビューコメントを確認
gh pr view <PR_NUMBER> --comments

# レビューの詳細（インラインコメント含む）
gh api repos/{owner}/{repo}/pulls/<PR_NUMBER>/comments --jq '.[].body'
```

**対応フロー:**
1. レビューコメントを確認（`sleep` で待たない、ノンブロッキングで確認）
2. レビューがまだ来ていなければ → 3-2 へ進み CI 状況を先に確認
3. 指摘が妥当なら修正をコミット & push（CI が再実行される）
4. 指摘が不要（false positive）ならスキップ

### 3-2: CI ステータスの確認

```bash
# PR のチェック状況を確認（ノンブロッキング）
gh pr checks <PR_NUMBER>

# 失敗ログの取得
gh run view <RUN_ID> --log-failed
```

**注意:** `sleep` や `gh pr checks --watch` で待たない。ノンブロッキングで確認し、pending なら他の作業を進める。

### 3-3: Lint / Test 失敗への対応

```bash
# 失敗した run のログを取得
gh run view <RUN_ID> --log-failed
```

**対応フロー:**
1. エラーログを読んで原因を特定
2. ローカルで修正
3. Phase 1 のローカル検証を再実行（該当チェックだけでOK）
4. コミット & push（CI が再実行される）

---

## Phase 4: マージ

**マージはユーザーが手動で行う。** Claude はマージコマンドを実行しない。

全チェック通過を確認したら、PR URL をユーザーに提示する。

```bash
# 全チェックが通っていることを確認
gh pr checks <PR_NUMBER>

# ユーザーに PR URL を提示
echo "全CI通過。マージお願いします: https://github.com/AddnessTech/addness-mcp/pull/<PR_NUMBER>"
```

### マージできない場合

- Required checks が通っていない → Phase 3 に戻る
- コンフリクト → ローカルで `git rebase main` して force push

---

## Phase 4.5: タグ付け

マージ後、適切なタグを付ける。

```bash
# main に切り替えて最新を取得
git checkout main && git pull origin main

# 直近のタグを確認してバージョンを決定
git tag --sort=-v:refname | head -5

# タグ付け & push（プレフィックスは変更領域に合わせる）
# MCP 変更: mcp-vX.Y.Z
# API 変更: vX.Y.Z
git tag -a <TAG> -m "<タグの説明>"
git push origin <TAG>
```

### バージョニング規約

- **patch (X.Y.Z+1)**: バグ修正、小さな改善
- **minor (X.Y+1.0)**: 新機能追加
- **major (X+1.0.0)**: 破壊的変更

---

## Phase 5: Addness ゴール作成

タグ付け完了後、対応内容を Addness にゴールとして記録し、即完了にする。

1. 適切な親ゴールを特定する（`list_my_goals` で確認）
2. `create_goal` でゴール作成（タイトルにタグバージョンを含める、description に PR 番号を含める）
3. `complete_goal` で即完了にする

```
# 例
create_goal:
  title: "get_goalレスポンスにゴールメンバー情報を追加 (mcp-v0.4.2)"
  parent_id: <親ゴールの short ID>
  description: "変更内容の要約。PR #XXXX"
  status: IN_PROGRESS

complete_goal:
  goal_id: <作成したゴールの short ID>
```

---

## Phase 6: Slack リリース通知

マージ＆タグ付け完了後、`times_nisshi` チャンネルにリリース通知を投稿する。

### 投稿フォーマット

```
<!here> <変更の要約タイトル> (<タグバージョン>)

やったこと:
- <変更点1>
- <変更点2>
- <変更点3>

<一言まとめ>

アップデート方法：
\`\`\`
curl -sL https://raw.githubusercontent.com/AddnessTech/addness-mcp/main/install.sh -q .content | base64 -d | bash
\`\`\`

PR: <PR URL>
```

### 例

```
<!here> MCPコメントのメンション展開リリースしました (mcp-v0.6.1)

やったこと:
- コメント本文中の `@短縮ID` をフルUUIDに自動展開するようにした
- `add_comment` / `update_comment` の両方に対応
- フルUUID・未知IDはそのままスルーで安全

これでClaude Codeからコメント投稿時に `@e79f0f16` のような短縮IDでメンションを書いても、バックエンドが正しくメンションとして認識するようになりました。

アップデート方法：
\`\`\`
curl -sL https://raw.githubusercontent.com/AddnessTech/addness-mcp/main/install.sh -q .content | base64 -d | bash
\`\`\`

PR: https://github.com/AddnessTech/addness-mcp/pull/2501
```

### 投稿方法

`slack_send_message` ツールを使用:
- channel: `times_nisshi` を `slack_search_channels` で検索して channel ID を取得
- text: 上記フォーマットに沿ったメッセージ

### 注意

- MCP 変更や新機能リリースなど、チームに共有すべき変更の場合に投稿する
- 小さなバグ修正やリファクタリングでは不要
- **敬語は不要**（自分の times チャンネル）

---

## 全体ループのまとめ

```
Phase 1: ローカル検証
  ↓ 全チェック通過
Phase 2: PR作成
  ↓ PR作成完了
Phase 3: レビュー対応 ←─────┐
  ↓ 指摘あり → 修正 → push ─┘
  ↓ 全チェック通過
Phase 4: マージ
  ↓ 完了
Phase 4.5: タグ付け
  ↓ タグ push 完了
Phase 5: Addness ゴール作成 & 完了
  ↓ 記録完了
Phase 6: Slack リリース通知
  ↓ 投稿完了
🎉 Ship complete!
```

---

## Phase 1.5: MCP 変更時のローカル検証

`` に変更がある場合、Phase 1 の後に**実際にMCPサーバーを動かして動作確認する**。

### 1. バイナリビルド & デプロイ

```bash
go build -o /tmp/addness-mcp-new ./

# 旧バイナリをバックアップして差し替え
cp ~/.local/bin/addness-mcp ~/.local/bin/addness-mcp.bak
cp /tmp/addness-mcp-new ~/.local/bin/addness-mcp
chmod +x ~/.local/bin/addness-mcp
```

### 2. 古いプロセスを kill

```bash
# 残留プロセスがあると /mcp reconnect が失敗する
ps aux | grep addness-mcp | grep -v grep
kill <PID>  # プロセスがあれば kill
```

### 3. ユーザーに `/mcp` を促す

バイナリデプロイ＋プロセス kill まで完了したら、ユーザーに `/mcp` を実行するよう伝える。
ユーザーが自分で再接続する。**自動では再接続できない。**

**`Failed to reconnect` の場合:**
- 残留プロセスが原因のことが多い → Step 2 をやり直す
- バイナリが壊れている可能性 → `addness-mcp.bak` に戻して再接続テスト

### 4. MCPツール呼び出しで実機テスト

`/mcp` 再接続後、**MCPツールを直接呼び出して動作確認する**。
ToolSearch で対象ツールのスキーマを取得し、実際に叩く。

```
# 基本フロー
1. list_members でキャッシュ充填（短縮ID → UUID マッピング）
2. list_my_goals で自分のゴール一覧取得
3. 変更したツールを実際に呼び出して動作確認
4. テストデータは後で削除する（テストコメント等）
```

**例: コメントメンション展開のテスト**
```
1. list_members → キャッシュ充填
2. list_my_goals → テスト対象ゴール取得
3. add_comment(goal_id=<short>, content="@<member_short_id> テスト")
4. list_comments(goal_id=<short>) → メンションが @名前 に展開されていることを確認
5. delete_comment(comment_id=<short>) → テストデータ削除
```

### 5. 問題なければ Phase 2 へ

バックアップバイナリは残しておく（ロールバック用）。

---

## MCP サーバー設定リファレンス

```bash
# MCP サーバー設定の確認
claude mcp get addness

# バイナリパス: ~/.local/bin/addness-mcp
# キャッシュ: ~/Library/Caches/addness-mcp/
#   - shortid_cache.json  (short ID ↔ UUID マッピング)
#   - session.json         (orgID, memberID の永続化)
```

---

## 注意事項

- **main への直接 push は禁止**。必ず PR 経由
- **`.claude/skills/` は gitignore 対象**。skill ファイルの変更はコミット不要、ブランチ切り替えも不要。直接編集してOK
- Claude PR Review が API limit で skip された場合は advisory 扱い（マージ可）
- Migration Check は `internal/migrations/` に変更がある場合のみ実行される
- CI の待ち時間中は他の作業を進めてよい
- **MCP テスト時は自分のデータ＋他メンバーのデータの両方で動作確認する**（他メンバー向け機能は他メンバーのIDで検証しないと意味がない）
