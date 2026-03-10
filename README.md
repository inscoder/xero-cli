# xero

`xero` is a terminal-first Go CLI for Xero with browser OAuth, persisted session state, tenant selection, and an MVP invoice listing command.

## Commands

```bash
xero auth login
xero auth status
xero auth logout
xero invoices --status AUTHORISED --limit 20
xero invoices --tenant <tenant-id> --json
xero doctor
```

## Configuration

- precedence: `flags > env vars > persisted config`
- config file: `~/.config/xero/config.json`
- session metadata: `~/.config/xero/session.json`
- token storage: `~/.config/xero/tokens.json` with `0600` permissions for MVP

### Environment variables

```bash
export XERO_AUTH_CLIENT_ID="your-client-id"
export XERO_AUTH_CLIENT_SECRET="your-client-secret"
export XERO_TENANT="your-default-tenant-id"
export XERO_AUTH_OPEN_COMMAND="open"
```

## Auth flow

`xero auth login` starts a loopback OAuth callback on `127.0.0.1`, opens the system browser, exchanges the authorization code using PKCE S256, discovers available tenants, and persists the chosen default tenant for later commands.

Refresh is gated by the stored token `generatedAt` timestamp. The CLI refreshes only when the token is older than 25 minutes.

## Output modes

- default: human-readable table output on stdout
- `--json`: stable JSON envelope on stdout
- `--quiet`: raw `data` payload only on stdout
- diagnostics, prompts, and progress always go to stderr

## Development

```bash
go test ./...
go test ./test/output -run TestWriteJSONEnvelopeContract
```

See `docs/auth.md`, `docs/commands/invoices.md`, and `docs/development/testing.md` for more detail.
