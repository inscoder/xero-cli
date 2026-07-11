# Browser OAuth

`xero login` uses Authorization Code + PKCE with a local callback on `http://localhost:8742/callback`.

## Setup

Create a profile with your Xero OAuth PKCE app client ID:

```bash
xero profile add my-company --client-id YOUR_CLIENT_ID
xero login -p my-company
```

The first profile becomes the default. API commands accept `-p, --profile`, and `XERO_PROFILE` can select a profile from the environment. `xero login --client-id <id>` or `XERO_CLIENT_ID=<id> xero login` can be used for one-off login setup, but named profiles are recommended.

## Behavior

- generates a PKCE verifier and `state`
- listens on `localhost:8742` and accepts both IPv4 and IPv6 loopback traffic when available
- opens the system browser to Xero login
- validates `state` on callback before exchanging the code
- discovers connected Xero tenants from `/connections`
- prompts for a tenant when multiple tenants are returned
- encrypts access and refresh tokens in `~/.config/xero/tokens.json`, keyed by profile
- stores the selected tenant with the active profile token
- writes non-secret metadata to `~/.config/xero/session.json`
- loads a local `.env` file from the current working directory when present
- uses default Xero CLI scopes unless `--scope` or `XERO_SCOPES` overrides them

## Token Storage

Tokens are encrypted with AES-256-GCM before writing `~/.config/xero/tokens.json`. Token entries are keyed by profile name, so each profile can have its own cached access token, refresh token, expiry, and selected Xero tenant.

Token storage has two parts:

- `~/.config/xero/tokens.json`: encrypted OAuth token cache
- encryption key source: either a local key file or a key derived from `XERO_TOKEN_PASSPHRASE`

### Default File Key

If no token storage environment variables are set, the CLI uses file-backed key storage:

```bash
xero login -p my-company
```

The CLI:

- generates a random 32-byte encryption key
- stores it at `~/.config/xero/.encryption-key` with `0600` permissions
- encrypts access and refresh tokens into `~/.config/xero/tokens.json`
- reuses `.encryption-key` for later commands

This is the most convenient mode. Its tradeoff is that any process running as your user that can read `.encryption-key` can decrypt `tokens.json`.

### Passphrase-Derived Key

If `XERO_TOKEN_PASSPHRASE` is set, it takes precedence over file-backed key storage:

```bash
export XERO_TOKEN_PASSPHRASE='a strong passphrase'
xero login -p my-company
```

The CLI:

- derives the encryption key from your passphrase using `scrypt`
- stores only a local salt at `~/.config/xero/.token-salt`
- does not need `~/.config/xero/.encryption-key`
- encrypts tokens into `~/.config/xero/tokens.json`

You must set the same passphrase for every later command that needs tokens:

```bash
export XERO_TOKEN_PASSPHRASE='a strong passphrase'
xero invoices -p my-company
```

If the passphrase is missing or different, the CLI cannot decrypt the token cache. Use this mode when you want stronger protection than a key file and are comfortable managing a secret environment variable.

### `XERO_KEY_STORAGE`

`XERO_KEY_STORAGE` controls the file-key path when `XERO_TOKEN_PASSPHRASE` is not set.

Supported values in this Go CLI today:

| Value | Behavior |
| --- | --- |
| unset / `auto` | Use `~/.config/xero/.encryption-key` |
| `file` | Explicitly use `~/.config/xero/.encryption-key` |
| `keyring` | Not implemented yet; returns an error |

These two commands are equivalent today:

```bash
xero login -p my-company
```

```bash
XERO_KEY_STORAGE=file xero login -p my-company
```

Do not use `XERO_KEY_STORAGE=keyring` yet:

```bash
XERO_KEY_STORAGE=keyring xero login -p my-company
```

It will fail and ask you to use `XERO_TOKEN_PASSPHRASE` or `XERO_KEY_STORAGE=file`. OS keyring support is reserved for a future implementation.

### Changing Storage Mode

Changing storage mode after login can make existing `tokens.json` unreadable unless the same encryption key can be recreated. If you switch from file key to passphrase, change passphrases, or otherwise cannot decrypt the token cache, re-authenticate the profile:

```bash
xero logout -p my-company
xero login -p my-company
```

For a simple setup, use the default file key. For stronger local protection, use `XERO_TOKEN_PASSPHRASE` and ensure it is set before every command that reads tokens.

## Refresh Policy

- the CLI persists `expiresAt` on every token write
- refresh runs when the token is within one minute of expiry
- non-interactive commands fail cleanly when refresh cannot recover the session
- interactive commands may re-authenticate in the browser if refresh fails and `--no-browser` is not set

## Troubleshooting

- missing client ID: run `xero profile add <name> --client-id <id>` or pass `--client-id` to `xero login`
- local development: copy `.env.example` to `.env` and set `XERO_PROFILE`; set `XERO_CLIENT_ID` only for login setup if needed
- invalid scope for client: pass `--scope` or set `XERO_SCOPES` to the exact scopes allowed by your Xero app
- Linux browser launch: the CLI uses `xdg-open` by default; override with `XERO_AUTH_OPEN_COMMAND` if your distro uses a different opener
- callback timeout: verify the browser can reach `http://localhost:8742/callback` and that port `8742` is free
- wrong organisation: run `xero login -p <profile>` and select the intended tenant, or use a different profile
- token storage: inspect `~/.config/xero/tokens.json` and `.encryption-key` permissions and rerun `xero doctor`

## Accounting write scopes

Invoice and bill create/update commands require the granular `accounting.invoices` scope. Attachment upload additionally requires `accounting.attachments`. Contact create, update, and archive commands require `accounting.contacts`; a list-only profile may instead use `accounting.contacts.read`. A typical write-capable override is:

```bash
export XERO_SCOPES="accounting.invoices accounting.attachments accounting.contacts"
xero login -p my-company
```

OAuth grants are stored in the profile token. Changing `XERO_SCOPES`, `.env`, or the login `--scope` flag does not expand an existing token, so log in again after changing the requested scopes. A 403 response is reported as `PermissionDenied` (exit code 21); verify both the app's allowed scopes and the scopes granted to that profile.
