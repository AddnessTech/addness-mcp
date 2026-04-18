# Addness MCP Server

REST APIを叩くMCPクライアント。Addness の API を MCP (Model Context Protocol) ツールとして提供する。

## Architecture

- Single Go package (root)
- Uses [mcp-go](https://github.com/mark3labs/mcp-go) SDK
- stdio transport

## Key Files

| File | Role |
|------|------|
| `main.go` | Tool registration, server setup |
| `client.go` | HTTP client for Addness API |
| `format.go` | Response parsing / formatting |
| `shortid.go` | Short ID mapping (`ids.Shorten()` / `ids.Resolve()`) |
| `login.go` | Authentication flow |
| `tools_*.go` | Tool implementations (goal, comment, activity, etc.) |

## Build & Test

```bash
# Build
go build .

# Test
go test . -race

# Lint (if available)
golangci-lint run .

# Format
goimports -w -local github.com/AddnessTech .
```

## Conventions

### Tool function pattern

Each tool file follows this pattern:
- `xxxTool()` — returns `mcp.Tool` definition
- `handleXxx()` — returns the handler function

### Response helpers

- `textResult()` / `errResult()` — standard MCP response wrappers

### ID handling

- `ids.Shorten()` — UUID to short ID (user-facing)
- `ids.Resolve()` — short ID back to UUID (API calls)

### API response handling

- `unwrapData()` — unwrap V2 API response envelope

### Tool descriptions

- Keep `description` in Japanese for user-facing tool descriptions
- Use `definition_of_done` (not `description`) for goal DoD

## Rules

- Never commit secrets or `.env` files
- Never push directly to main — always use PRs
