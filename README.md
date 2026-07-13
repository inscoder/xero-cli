# xero

`xero` is a terminal-first Go CLI for Xero with browser OAuth, named profiles, encrypted token storage, contact management, invoice and bill listing and mutation workflows, attachment upload, invoice approval, invoice PDF download, and online invoice URL lookup.

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
xero profile add my-company --client-id YOUR_CLIENT_ID
xero profile list
xero profile set-default my-company
xero login
xero logout
xero version
xero contacts list --search "Acme"
xero contacts create --name "Acme Corp" --email acme@example.com --phone "+1234567890"
xero contacts update --contact-id 00000000-0000-0000-0000-000000000001 --name "Acme Corporation"
xero invoices --status AUTHORISED,PAID --page 1 --page-size 100
xero invoices --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --order "UpdatedDateUTC DESC"
xero bills --status AUTHORISED --where 'AmountDue>=5000'
xero invoices create --file docs/examples/invoice-create.json
xero bills create --file docs/examples/bill-create.json
xero invoices update --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --file invoice-update.json
xero bills update --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --file bill-update.json
xero invoices attachments upload --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --file receipt.pdf
xero bills attachments upload --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --file receipt.pdf
xero invoices approve --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices --profile my-company --json
xero doctor
```

## Configuration

- precedence: `flags > env vars (including local .env) > persisted config`
- config file: `~/.config/xero/config.json`
- session metadata: `~/.config/xero/session.json`
- encrypted token storage: `~/.config/xero/tokens.json` with `0600` permissions
- token encryption key: `~/.config/xero/.encryption-key` unless `XERO_TOKEN_PASSPHRASE` is set

In normal usage, the CLI reads `~/.config/xero/config.json` for profiles, default profile, and output mode. Tokens are encrypted and cached per profile in `~/.config/xero/tokens.json`. For development convenience, it also loads a local `.env` file from the current working directory when present.

### Environment variables

```bash
export XERO_PROFILE="my-company"
export XERO_CLIENT_ID="your-client-id" # optional for xero login setup
export XERO_SCOPES="accounting.invoices accounting.attachments accounting.contacts"
export XERO_AUTH_OPEN_COMMAND="xdg-open" # Linux; use "open" on macOS
export XERO_TOKEN_PASSPHRASE="optional-token-encryption-passphrase"
```

You can also copy `.env.example` to `.env` for local development.

Normal API commands use the active profile selected by `-p, --profile` or `XERO_PROFILE`. `XERO_CLIENT_ID` is useful for `xero login` setup when you do not want to save a profile first, but named profiles are recommended for regular use.

`xero login` requests the default Xero CLI scope set unless `--scope` or `XERO_SCOPES` is provided. Required OAuth scopes (`openid`, `profile`, `email`, `offline_access`) are always prepended automatically.

## Auth flow

`xero login` starts a local OAuth callback on `http://localhost:8742/callback`, opens the system browser, exchanges the authorization code using PKCE S256, and listens on both IPv4 and IPv6 loopback addresses when available before saving the selected tenant with the active profile token.

On SSH and headless Linux sessions, the CLI automatically prints the authorization URL instead. Open it in a browser, complete Xero authentication, then copy the full `http://localhost:8742/callback?...` URL from the browser address bar and paste it into the terminal. Use `xero login --no-browser` (or `XERO_AUTH_NO_BROWSER=true`) to force this flow in any environment.

Use profiles to switch organisations or OAuth apps: `xero invoices -p my-company`. There is no temporary tenant override; login/profile selection controls the connected organisation.

Xero access tokens are short-lived, typically 30 minutes. The CLI refreshes when the stored `expiresAt` is within one minute of expiry.

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

`xero invoices` lists sales invoices (`ACCREC`) and `xero bills` lists purchase bills (`ACCPAY`). Both commands call Xero's `GET /Invoices` endpoint and apply the correct Xero `Type` filter automatically.

`xero contacts` provides explicit `list`, `create`, and `update` subcommands. Contact updates are partial scalar updates; archiving is a status-only operation and requires `--confirm-archive`. Contact write commands require `accounting.contacts`, while list-only profiles may use `accounting.contacts.read`.

`create` and `update` accept one strict camelCase JSON object through `--file PATH|-`. The command namespace owns the Xero type: `invoices` always sends `ACCREC`, while `bills` always sends `ACCPAY`. Updates containing `lineItems` require `--replace-line-items` because Xero treats the submitted array as the complete desired set. Every mutation supports `--idempotency-key`; when omitted, the CLI generates and reports one.

`attachments upload` accepts a regular local file up to 10,000,000 bytes. A same-name attachment is never replaced implicitly: pass `--overwrite` only after the preflight confirms the filename already exists. Sales-invoice uploads additionally support `--include-online` for new attachments.

Invoice and bill listing requests default to `page=1` so commands do not ask Xero for an unbounded result set. Use `--page` and `--page-size` to control pagination explicitly.

`xero invoices online-url` uses Xero's dedicated online-invoice endpoint. It does not reuse the invoice `url` field returned by `xero invoices`, because Xero documents that field as a source-document link inside Xero rather than the customer-facing online invoice URL.

`xero invoices approve` is the first invoice write command. It approves exactly one invoice by setting its status to `AUTHORISED`, includes the resolved tenant in structured output, and is intended for explicit single-resource use. Required Xero scopes are `accounting.transactions` for legacy apps or `accounting.invoices` for granular-scope apps.

`xero invoices pdf` is the CLI's binary download path. It requires an explicit `--output` destination, returns saved-file metadata for `--json` and `--quiet`, and only streams raw PDF bytes when `--output -` is used in a non-interactive context.

## Development

```bash
go test ./...
go test ./test/output -run TestWriteJSONEnvelopeContract
```

See `docs/auth.md`, `docs/commands/contacts.md`, `docs/commands/invoices.md`, `docs/commands/bills.md`, and `docs/development/testing.md` for more detail.

## Releasing

Create an annotated semver tag such as `v0.1.0` and push it to GitHub. The release workflow will run tests, build archives with GoReleaser, publish the GitHub Release, and attach `checksums.txt`.

See `RELEASING.md` for the full checklist.
