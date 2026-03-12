# Browser OAuth

`xero auth login` uses Authorization Code + PKCE with a local callback on `http://localhost:3000/callback`.

## Behavior

- generates a PKCE verifier and `state`
- listens on `localhost:3000` and accepts both IPv4 and IPv6 loopback traffic when available
- opens the system browser to Xero login
- validates `state` on callback before exchanging the code
- stores tokens in `~/.config/xero/tokens.json` with `0600` permissions for MVP
- writes non-secret metadata to `~/.config/xero/session.json`
- prompts for a default tenant when multiple tenants are returned
- loads a local `.env` file from the current working directory when present, which is useful for local development secrets
- requires explicit scopes through `XERO_AUTH_SCOPES` or a `scopes` array in `~/.config/xero/config.json`

## Refresh policy

- the CLI persists `generatedAt` on every token write
- refresh runs only when `now - generatedAt > 25m`
- non-interactive commands fail cleanly when refresh cannot recover the session
- interactive commands may re-authenticate in the browser if refresh fails and `--no-browser` is not set

## Troubleshooting

- missing client ID: set `XERO_AUTH_CLIENT_ID`
- local development: copy `.env.example` to `.env` and set `XERO_AUTH_CLIENT_ID` / `XERO_AUTH_CLIENT_SECRET`
- invalid scope for client: set `XERO_AUTH_SCOPES` or `~/.config/xero/config.json` `scopes` to the exact scopes allowed by your Xero app
- missing scopes: the CLI will refuse login until you set `XERO_AUTH_SCOPES` or configure `scopes` in `~/.config/xero/config.json`
- Linux browser launch: the CLI uses `xdg-open` by default; override with `XERO_AUTH_OPEN_COMMAND` if your distro uses a different opener
- callback timeout: verify the browser can reach `http://localhost:3000/callback` and that port `3000` is free
- stale tenant: rerun `xero auth login` to discover and choose a valid tenant again
- token storage: inspect `~/.config/xero/tokens.json` permissions and rerun `xero doctor`
