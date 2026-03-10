---
title: feat: Build Xero CLI with browser auth and invoice listing
type: feat
status: completed
date: 2026-03-10
---

# feat: Build Xero CLI with browser auth and invoice listing

## Overview

Create a standalone `xero` CLI in Go, inspired by Basecamp CLI, that calls Xero's REST APIs directly, adds browser-based OAuth support for all regions, and ships an MVP read command: `xero invoices`.

The first release should optimize for reliable authentication, predictable terminal/JSON output, and a narrow scope that proves the CLI shape before expanding to more Xero resources.

## Problem Statement

The Xero APIs already support invoice listing and OAuth flows, but there is no opinionated user-facing CLI for them. The current auth options still create adoption friction:

- Custom Connections require a paid setup and are limited to a subset of regions.
- Bearer-token mode is short-lived and pushes token lifecycle management to the user.
- There is no opinionated CLI UX for interactive login, tenant selection, script-safe JSON output, or invoice listing from a terminal.

This feature creates a simpler product path: a terminal-native CLI that can log in through the browser, persist auth state, select a Xero tenant, and list invoices with a stable output contract.

## Proposed Solution

Build a new CLI application named `xero` with these initial capabilities:

1. `xero auth login` performs browser-based OAuth using Authorization Code + PKCE with a loopback redirect.
2. `xero auth status` reports auth health, active auth mode, token state, and default tenant without exposing secrets.
3. `xero auth logout` clears persisted auth state.
4. `xero invoices` lists invoices by calling the Xero Accounting API behind a CLI-friendly command contract.
5. `xero doctor` validates config, token storage, browser prerequisites, and tenant/default state.

The CLI should follow Basecamp CLI design patterns where they fit:

- noun-first commands
- human-readable output by default
- `--json` and `--quiet` for automation
- stable stdout, diagnostics on stderr
- breadcrumbs or next-step hints in JSON/human output

## Technical Approach

### Architecture

Recommended implementation shape:

- Build the CLI as a direct Xero API client with a small internal service layer instead of routing through an MCP server.
- Reuse the browser-auth direction from Xero MCP PR #115 as inspiration for the auth foundation, but implement it natively in Go for this CLI.
- Keep Xero-specific session concerns in a dedicated auth/config layer so future commands can share tenant and token state.
- Use `cobra` for command structure/help output and `viper` for config loading, env binding, and precedence management.

Suggested initial project layout:

```text
cmd/xero/main.go
internal/commands/auth_login.go
internal/commands/auth_status.go
internal/commands/auth_logout.go
internal/commands/invoices_list.go
internal/commands/doctor.go
internal/auth/browser_oauth.go
internal/auth/token_store.go
internal/auth/tenant_store.go
internal/config/config.go
internal/xeroapi/client.go
internal/output/human.go
internal/output/json.go
internal/errors/exit_codes.go
internal/version/version.go
test/auth/browser_oauth_test.go
test/commands/invoices_test.go
test/output/json_contract_test.go
test/integration/xero_invoices_integration_test.go
```

Recommended boundaries:

- `cmd/xero/main.go`: bootstrap the root Cobra command and shared flags
- `internal/auth/browser_oauth.go`: PKCE, state generation, loopback callback, browser open/fallback URL flow
- `internal/auth/token_store.go`: persisted token load/save/refresh with locking and generated-at timestamps
- `internal/auth/tenant_store.go`: tenant discovery, default tenant persistence, override resolution
- `internal/xeroapi/client.go`: one integration layer for invoking Xero REST endpoints such as `GET /api.xro/2.0/Invoices`
- `internal/output/*.go`: one place for table formatting and JSON envelope stability
- `internal/errors/exit_codes.go`: typed CLI exits for auth/config/network/API/validation failure

### Command Surface

MVP command set:

```bash
xero auth login
xero auth status
xero auth logout
xero doctor
xero invoices
```

Recommended `xero invoices` arguments for MVP:

- `--tenant <tenant-id>`: override saved default tenant
- `--status <status>`: filter invoice status
- `--contact <name-or-id>`: narrow results when supported by the Xero API query mapping
- `--since <YYYY-MM-DD>`: filter recent invoices
- `--page <n>`: explicit page number
- `--limit <n>`: page size / display limit
- `--json`: structured output envelope
- `--quiet`: raw `data` only for scripts
- `--no-browser`: fail instead of opening a browser when auth is required

Default behavior:

- `xero invoices` should attempt to use persisted auth and default tenant.
- If no auth exists, interactive terminals should guide the user to `xero auth login` or optionally prompt to authenticate.
- In non-interactive mode, the command must fail with a typed error instead of launching a browser.

### Authentication Design

Browser auth should follow desktop-app best practices:

- Use Authorization Code + PKCE (`S256`).
- Use loopback redirect on `127.0.0.1` with an ephemeral port.
- Validate `state` on callback.
- Open the system browser; if that fails, print a one-time auth URL and wait for callback or pasted code.
- Persist each token's generated date/time and only refresh when the current token is older than 25 minutes, since the token lifetime is 30 minutes.
- If refresh fails, interactive sessions may re-authenticate; non-interactive sessions must fail cleanly.

Token and config strategy:

- For MVP, persist refresh and access tokens in a documented file under `~/.config/xero/` with restricted permissions.
- Keep config and secrets separate.
- Define precedence as `flags > env vars > persisted config`.

Recommended persisted files:

```text
~/.config/xero/config.json          # defaults, output mode, tenant preference
~/.config/xero/session.json         # non-secret auth metadata only
~/.config/xero/tokens.json          # refresh/access tokens for MVP, written with restricted permissions
```

Document the file path and permission model explicitly for MVP.

### Tenant Handling

Tenant selection is a first-class requirement, not an afterthought.

The CLI should:

- discover available tenants after successful browser auth
- prompt the user to choose a default tenant when multiple are available
- persist the chosen tenant in config
- allow `--tenant` to override config per command
- detect revoked or missing tenants and force reselection with a clear error path

### Output Contract

Human output should be optimized for terminal reading. JSON output should be optimized for scripts.

Recommended JSON envelope:

```json
{
  "ok": true,
  "data": [
    {
      "invoiceId": "...",
      "invoiceNumber": "INV-0001",
      "contactName": "Acme Ltd",
      "status": "AUTHORISED",
      "total": 123.45,
      "amountDue": 23.45,
      "currency": "USD",
      "dueDate": "2026-03-10",
      "updatedAt": "2026-03-09T12:30:00Z"
    }
  ],
  "summary": "12 invoices",
  "breadcrumbs": [
    {
      "action": "show",
      "cmd": "xero invoices --tenant <tenant-id> --json"
    }
  ]
}
```

Rules:

- stdout contains only command output
- stderr contains progress, warnings, and diagnostics
- `--json` never emits ANSI styling
- `--quiet` emits raw `data` only
- empty result sets are valid, not errors

### Implementation Phases

#### Phase 1: CLI foundation and auth contract

Deliverables:

- scaffold command runner in `cmd/xero/main.go`
- add auth commands in `internal/commands/auth_*.go`
- implement browser login in `internal/auth/browser_oauth.go`
- define token/config persistence in `internal/auth/token_store.go` and `internal/config/config.go`
- wire shared flags, env binding, and config precedence through `cobra` + `viper`
- add exit code taxonomy in `internal/errors/exit_codes.go`

Success criteria:

- user can log in through browser auth
- auth state persists across CLI runs
- token refresh works without manual action and only triggers when the stored token age is greater than 25 minutes
- tenant discovery works after login

Estimated effort:

- 3-5 engineering days

#### Phase 2: invoice command MVP

Deliverables:

- implement `xero invoices` in `internal/commands/invoices_list.go`
- add direct Xero API client in `internal/xeroapi/client.go`
- implement output formatting in `internal/output/human.go` and `internal/output/json.go`
- add command tests in `test/commands/invoices_test.go`
- add one integration path in `test/integration/xero_invoices_integration_test.go`

Success criteria:

- command lists invoices for a chosen or default tenant
- filters behave predictably
- JSON output remains stable
- non-interactive auth failures do not launch a browser

Estimated effort:

- 2-4 engineering days

#### Phase 3: supportability and polish

Deliverables:

- implement `internal/commands/doctor.go`
- add corruption/recovery handling for bad config or token state
- document installation and usage in `README.md`
- add CI checks for lint, tests, and JSON contract snapshots

Success criteria:

- users can self-diagnose auth/config issues
- error messages prescribe next actions
- package is ready for broader MVP feedback

Estimated effort:

- 1-2 engineering days

## Alternative Approaches Considered

### 1. Route through the Xero MCP server

Rejected because this CLI should talk to Xero directly rather than depending on MCP as a bridge layer.

### 2. Ship only Custom Connections support first

Rejected because the request explicitly prioritizes browser-based OAuth, and Custom Connections exclude some regions and use cases.

### 3. Support only bearer-token passthrough in the CLI

Rejected because it creates poor user UX, offloads token rotation to the user, and does not solve the main onboarding problem.

### 4. Make `xero invoices` the only command with implicit auth behavior

Rejected because supportability suffers without explicit `auth` and `doctor` commands.

## System-Wide Impact

### Interaction Graph

`xero invoices` triggers command parsing in `cmd/xero/main.go`, which resolves config in `internal/config/config.go`, which loads auth state from `internal/auth/token_store.go`, which may refresh tokens through `internal/auth/browser_oauth.go`, which then passes a valid session and tenant to `internal/xeroapi/client.go`, which calls the Xero Accounting API invoices endpoint, which returns normalized invoice records to `internal/output/human.go` or `internal/output/json.go`.

`xero auth login` triggers browser auth in `internal/auth/browser_oauth.go`, which discovers tenants, which updates default tenant state in `internal/auth/tenant_store.go`, which persists defaults in `internal/config/config.go`, which affects future `xero invoices` calls.

### Error & Failure Propagation

Document and implement explicit classes for:

- `AuthRequiredError`
- `TokenRefreshFailedError`
- `TenantSelectionRequiredError`
- `ConfigCorruptedError`
- `XeroRequestError`
- `XeroApiError`
- `NetworkError`
- `RateLimitError`
- `ValidationError`

Each should map to a deterministic exit code and user-facing recovery guidance.

### State Lifecycle Risks

Primary risks:

- token refresh races when two CLI commands run at once
- default tenant becoming invalid after revocation
- corrupted config or token files leaving the CLI half-usable
- non-interactive sessions hanging while waiting for browser auth

Mitigations:

- lock token writes
- atomically replace persisted session data
- validate tenant availability before command execution
- time out auth callbacks and fail with actionable messages

### API Surface Parity

The plan should preserve conceptual parity between Xero resources and CLI verbs.

- Xero invoices resources map to CLI `xero invoices`
- future CLI resources should follow the same noun-first shape as Basecamp CLI
- any auth behavior added for CLI should remain compatible with Xero's OAuth contract and documented PKCE guidance

### Integration Test Scenarios

- log in via browser auth, select a tenant, then run `xero invoices --json`
- run `xero invoices` with a token younger than 25 minutes and verify no refresh occurs
- run `xero invoices` with a token older than 25 minutes and a valid refresh token
- run `xero invoices --no-browser --json` with no session and verify typed failure
- run `xero invoices` when saved tenant access has been revoked
- run concurrent invoice commands and verify token persistence remains valid

## Acceptance Criteria

### Functional Requirements

- [x] `xero auth login` opens the system browser using loopback OAuth on `127.0.0.1` with PKCE S256 and validates `state`
- [x] successful login persists auth state for subsequent CLI runs
- [x] persisted auth state includes the token generated date/time used to decide refresh timing
- [x] successful login discovers Xero tenants and supports choosing a default tenant when multiple are available
- [x] `xero auth status` reports auth mode, token validity state, and default tenant without exposing secrets
- [x] `xero auth logout` clears persisted auth state safely
- [x] `xero invoices` lists invoices using a saved default tenant or `--tenant`
- [x] `xero invoices` supports the documented MVP filters: `--status`, `--contact`, `--since`, `--page`, `--limit`, `--tenant`
- [x] `xero invoices --json` emits stable JSON only on stdout
- [x] `xero invoices --quiet` emits raw data only on stdout
- [x] interactive sessions may recover from expired auth by re-authenticating; non-interactive sessions fail without opening a browser
- [x] token refresh is attempted only when the stored token generated date/time is more than 25 minutes old
- [x] `xero doctor` validates config, token storage, browser prerequisites, and tenant configuration

### Non-Functional Requirements

- [x] token storage uses a documented restricted-permission file under `~/.config/xero/` for MVP
- [x] config precedence is defined and implemented as `flags > env vars > persisted config` using `viper`
- [x] diagnostics are emitted on stderr and never contaminate JSON stdout
- [x] exit codes are stable and documented for auth, config, validation, network, and API failures
- [x] auth callback flow enforces timeout and cancellation behavior

### Quality Gates

- [x] unit coverage exists for auth state parsing, config precedence, tenant resolution, and output mapping
- [x] unit coverage verifies refresh gating based on stored token age thresholds, including under-25-minute and over-25-minute cases
- [x] integration coverage exists for login + invoice listing + refresh flow
- [x] JSON contract tests protect `--json` and `--quiet` output schemas
- [x] README usage examples include `xero auth login`, `xero invoices`, and `xero invoices --json`

## Success Metrics

- First-time interactive setup completes without manual token copying for a valid Xero user.
- Returning users can run `xero invoices` without re-authenticating in normal sessions.
- JSON consumers can rely on a stable envelope with no stderr leakage into stdout.
- MVP feedback confirms the command shape is clear enough to expand into more Xero resources.

## Dependencies & Prerequisites

- Access to Xero Accounting API documentation and OAuth integration requirements
- Adopt `cobra` + `viper` as the initial Go CLI stack and define module/package layout around it
- Xero OAuth application credentials suitable for browser auth
- Agreement on secure token storage strategy by platform
- Test Xero tenant(s) with invoice data for integration validation

## Risk Analysis & Mitigation

| Risk | Impact | Mitigation |
|---|---|---|
| Browser auth fails on headless or locked-down machines | onboarding failure | add `--no-browser`, manual URL fallback, and explicit non-interactive behavior |
| Token storage is insecure or corrupted | auth breakage / secret exposure | restrict file permissions, write atomically, and add corruption recovery |
| Tenant handling is ambiguous | wrong-org data access or command failure | force default tenant selection and allow per-command override |
| Xero API contract or filtering semantics change upstream | CLI breakage | isolate the Xero API client and pin/test against known endpoint behavior |
| Output contract churn breaks scripts | automation regressions | snapshot JSON schema and document compatibility expectations |
| Large invoice lists or pagination mismatch | incomplete data / UX confusion | define page/limit semantics early and test against real tenants |

## Resource Requirements

- One engineer familiar with Go CLI tooling and OAuth flows
- Access to at least one test Xero organization with sample invoices
- Optional design input for terminal output polish, but not required for MVP

## Future Considerations

- add `xero invoices show <id>` after MVP stability
- add other direct API-backed nouns such as `contacts`, `accounts`, and `payments`
- support project-local defaults if teams want shared tenant/environment hints
- add shell completions and richer breadcrumbs once command surface expands
- consider packaging and distribution paths similar to Basecamp CLI after MVP validation

## Documentation Plan

Update or create:

- `README.md` for install, auth, commands, and output modes
- `docs/auth.md` for browser OAuth, token storage, tenant selection, and troubleshooting
- `docs/commands/invoices.md` for argument semantics and JSON examples
- `docs/development/testing.md` for local test setup with Xero credentials or mocks

Example snippet to include in `docs/commands/invoices.md`:

```bash
# docs/commands/invoices.md
xero auth login
xero invoices --status AUTHORISED --limit 20
xero invoices --tenant <tenant-id> --json
```

## Sources & References

### Internal References

- Local workspace review: `/Users/cesar/src/xero-cli` is currently empty, so this plan establishes initial conventions rather than extending existing ones.

### External References

- Basecamp CLI reference: https://github.com/basecamp/basecamp-cli
- Basecamp install and UX patterns: https://raw.githubusercontent.com/basecamp/basecamp-cli/main/install.md
- Xero Invoices API docs: https://developer.xero.com/documentation/api/accounting/invoices
- Xero OAuth overview: https://developer.xero.com/documentation/guides/oauth2/overview
- Xero PKCE flow: https://developer.xero.com/documentation/guides/oauth2/pkce-flow
- Xero MCP browser auth PR (reference only for auth UX ideas): https://github.com/XeroAPI/xero-mcp-server/pull/115
- Xero Custom Connections: https://developer.xero.com/documentation/guides/oauth2/custom-connections
- RFC 8252 OAuth for native apps: https://datatracker.ietf.org/doc/html/rfc8252
- RFC 7636 PKCE: https://datatracker.ietf.org/doc/html/rfc7636

### Related Work

- Xero's Accounting API already exposes invoice listing directly
- PR #115 documents a useful browser-auth shape: PKCE, token persistence, automatic refresh, and browser fallback behavior
