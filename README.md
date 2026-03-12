# xero

`xero` is a terminal-first Go CLI for Xero with browser OAuth, persisted session state, tenant selection, invoice listing, invoice approval, invoice PDF download, and online invoice URL lookup.

## Install

### Go install

```bash
go install github.com/inscoder/xero-cli/cmd/xero@latest
xero version
```

### GitHub Releases

Download the archive for your platform from the GitHub Releases page, unpack it, move `xero` onto your `PATH`, then verify the build:

```bash
xero version
```

Release assets are published for macOS, Linux, and Windows on every `v*` tag.

## Commands

```bash
xero auth login
xero auth status
xero auth logout
xero version
xero invoices --status AUTHORISED,PAID --page 1 --page-size 100
xero invoices --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --order "UpdatedDateUTC DESC"
xero invoices approve --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices --tenant <tenant-id> --json
xero doctor
```

## Configuration

- precedence: `flags > env vars (including local .env) > persisted config`
- config file: `~/.config/xero/config.json`
- persisted OAuth app credentials: `~/.config/xero/auth.json`
- session metadata: `~/.config/xero/session.json`
- token storage: `~/.config/xero/tokens.json` with `0600` permissions for MVP

In normal usage, the CLI reads `~/.config/xero/config.json` for non-secret persisted defaults like tenant and output mode, and `~/.config/xero/auth.json` for persisted OAuth client credentials needed for later token refresh. For development convenience, it also loads a local `.env` file from the current working directory when present.

### Environment variables

```bash
export XERO_AUTH_CLIENT_ID="your-client-id"
export XERO_AUTH_CLIENT_SECRET="your-client-secret"
export XERO_AUTH_SCOPES="openid profile email offline_access accounting.transactions accounting.contacts accounting.settings.read accounting.reports.read"
export XERO_TENANT="your-default-tenant-id"
export XERO_AUTH_OPEN_COMMAND="xdg-open" # Linux; use "open" on macOS
```

You can also copy `.env.example` to `.env` for local development.

You must set scopes explicitly with `XERO_AUTH_SCOPES` or add a `scopes` array to `~/.config/xero/config.json`; the CLI no longer assumes a default scope set.

## Auth flow

`xero auth login` starts a local OAuth callback on `http://localhost:3000/callback`, opens the system browser, exchanges the authorization code using PKCE S256, and listens on both IPv4 and IPv6 loopback addresses when available before persisting the chosen default tenant for later commands.

After a successful login, the CLI also persists the current OAuth client ID and client secret to `~/.config/xero/auth.json` so later refreshes still work in new shells where the original environment variables are no longer set.

Refresh is gated by the stored token `generatedAt` timestamp. The CLI refreshes only when the token is older than 25 minutes.

## Output modes

- default: human-readable table output on stdout
- `--json`: stable JSON envelope on stdout; on failure, emit `{ "ok": false, "error": ... }`
- `--quiet`: raw payload only on stdout; on failure, emit the raw error object without the outer envelope
- diagnostics, prompts, and progress always go to stderr

Machine-readable contract examples:

```json
{
  "ok": false,
  "error": {
    "kind": "XeroApiError",
    "message": "A validation exception occurred",
    "exitCode": 14
  }
}
```

```json
{
  "kind": "XeroApiError",
  "message": "A validation exception occurred",
  "exitCode": 14
}
```

`xero invoices online-url` uses Xero's dedicated online-invoice endpoint. It does not reuse the invoice `url` field returned by `xero invoices`, because Xero documents that field as a source-document link inside Xero rather than the customer-facing online invoice URL.

`xero invoices approve` is the first invoice write command. It approves exactly one invoice by setting its status to `AUTHORISED`, includes the resolved tenant in structured output, and is intended for explicit single-resource use. Required Xero scopes are `accounting.transactions` for legacy apps or `accounting.invoices` for granular-scope apps.

`xero invoices pdf` is the CLI's binary download path. It requires an explicit `--output` destination, returns saved-file metadata for `--json` and `--quiet`, and only streams raw PDF bytes when `--output -` is used in a non-interactive context.

## Development

```bash
go test ./...
go test ./test/output -run TestWriteJSONEnvelopeContract
```

See `docs/auth.md`, `docs/commands/invoices.md`, and `docs/development/testing.md` for more detail.

## Releasing

Create an annotated semver tag such as `v0.1.0` and push it to GitHub. The release workflow will run tests, build archives with GoReleaser, publish the GitHub Release, and attach `checksums.txt`.

See `RELEASING.md` for the full checklist.
