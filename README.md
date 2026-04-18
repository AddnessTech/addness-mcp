# Addness MCP Server

[Addness](https://www.addness.com) の MCP (Model Context Protocol) サーバー。Claude Code から Addness のゴール管理を操作できます。

## インストール

```bash
gh api repos/AddnessTech/addness-mcp/contents/install.sh -q .content | base64 -d | bash
```

## セットアップ

### 1. ログイン

```bash
addness-mcp login
```

### 2. Claude Code に MCP サーバーを追加

```bash
claude mcp add addness -- addness-mcp
```

### 3. 組織を選択

Claude Code で以下を実行:
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

## ライセンス

MIT
