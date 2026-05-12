# internal/gateway/ — TRANSITIONAL

This directory is **NOT in the v6 design spec** (§2.1.1).

The v6 spec defines `nodes/gateway_client.go` as the preferred way to interact with
the external `rivision_gateway` process via HTTP (§5.4 "★ Gateway API 客户端 (非内嵌)").

However, `cmd/hub/main.go` currently depends on three packages from this directory:
- `internal/gateway/auth` (node token authentication)
- `internal/gateway/nodes` (node registry, heartbeat)
- `internal/gateway/tasks` (task tracker)

These dependencies are embedded (in-process) rather than calling the external
`rivision_gateway` via HTTP, which is the v6 target architecture.

## Migration plan
1. Move node registration/heartbeat logic to use `nodes/gateway_client.go` HTTP calls
2. Move token auth to `internal/auth/` (user + node auth consolidated)
3. Move task tracking to a new `internal/tasks/` package or remove if not needed
4. Delete this directory once all references in `main.go` are migrated

## Do NOT delete this directory until main.go is refactored.
