# Addness MCP Server

[Addness](https://www.addness.com) の MCP (Model Context Protocol) サーバー。Claude Code から Addness のゴール管理を操作できます。
<img width="1122" height="1402" alt="ChatGPT Image 2026年4月22日 16_09_05" src="https://github.com/user-attachments/assets/5103fa52-2eb3-424f-b8b5-0a8022edbe63" />

## インストール

### macOS / Linux

```bash
curl -sL https://raw.githubusercontent.com/AddnessTech/addness-mcp/main/install.sh | bash
```

### Homebrew

```bash
brew install AddnessTech/tap/addness-mcp
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/AddnessTech/addness-mcp/main/install.ps1 | iex
```

### 手動インストール

[Releases](https://github.com/AddnessTech/addness-mcp/releases/latest) から OS/アーキテクチャに合ったバイナリをダウンロード:

| OS | Intel/AMD | Apple Silicon/ARM |
|---|---|---|
| macOS | `addness-mcp-darwin-amd64` | `addness-mcp-darwin-arm64` |
| Linux | `addness-mcp-linux-amd64` | `addness-mcp-linux-arm64` |
| Windows | `addness-mcp-windows-amd64.exe` | `addness-mcp-windows-arm64.exe` |

## セットアップ

### 1. ログイン

```bash
addness-mcp login
```

### 2. MCP クライアントに追加

ログイン完了後、以下のようなコマンドが表示されます:

```bash
claude mcp add -t stdio -e ADDNESS_API_TOKEN='sk-...' -e ADDNESS_API_URL='https://vt.api.addness.com' -s user addness -- '/path/to/addness-mcp'
```

**Claude Code** の場合はこのコマンドをそのまま実行してください。

**Codex** の場合は `~/.codex/config.toml` に追加:
```toml
[mcp_servers.addness]
command = "/path/to/addness-mcp"
env = { ADDNESS_API_URL = "https://vt.api.addness.com", ADDNESS_API_TOKEN = "sk-..." }
```

**その他の MCP クライアント（Cursor, Windsurf, VS Code 等）** の場合は MCP 設定に追加:
```json
{
  "mcpServers": {
    "addness": {
      "command": "/path/to/addness-mcp",
      "env": {
        "ADDNESS_API_URL": "https://vt.api.addness.com",
        "ADDNESS_API_TOKEN": "sk-..."
      }
    }
  }
}
```

### 3. 組織を選択

MCP クライアントで以下を実行:
```
list_organizations → switch_organization
```

## 利用可能なツール

| カテゴリ | ツール |
|---|---|
| **認証** | `auth_login` |
| **組織** | `list_organizations`, `switch_organization`, `list_members` |
| **通知** | `list_notifications`, `mark_notifications_read` |
| **ゴール** | `list_my_goals`, `get_goal`, `create_goal`, `update_goal`, `complete_goal`, `delete_goal`, `move_goal`, `reorder_goal`, `list_member_goals`, `get_goal_ancestors` |
| **検索・階層** | `search_goals`, `list_subgoals` |
| **アーカイブ** | `archive_goal`, `unarchive_goal` |
| **コメント** | `list_comments`, `add_comment`, `update_comment`, `delete_comment`, `resolve_comment`, `toggle_reaction` |
| **アサイン** | `assign_member`, `unassign_member`, `list_assignments` |
| **招待** | `invite_members`, `create_invite_link`, `list_invite_links`, `list_invited_members` |
| **定常ゴール** | `set_recurring`, `remove_recurring`, `get_recurring` |
| **今日のゴール** | `list_todays_goals`, `get_goal_history` |
| **アクティビティ** | `get_member_activity`, `get_goal_activity`, `get_activity_summary` |

## 環境変数

| 変数 | 説明 | デフォルト |
|---|---|---|
| `ADDNESS_API_URL` | API のベース URL | `https://api.addness.com` |
| `ADDNESS_API_TOKEN` | API トークン（`addness-mcp login` で自動設定） | - |

## セキュリティについて

このリポジトリはpublicですが、セキュリティ上の問題はありません。

- **シークレットは含まれていません** — APIトークンは各ユーザーがローカルで `addness-mcp login` により設定します
- **バックエンドのコードは含まれていません** — このリポジトリはREST APIを叩くMCPクライアントのみです
- **APIエンドポイントのパス情報のみ** — リバースエンジニアリングでも取得可能な情報です
- **認証はAPIキーで保護されています** — MCPサーバーを動かしても、認証なしではデータにアクセスできません

## ライセンス

MIT
