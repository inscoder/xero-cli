# Browser OAuth

`xero auth login` uses Authorization Code + PKCE with a loopback callback on `127.0.0.1`.

## Behavior

- generates a PKCE verifier and `state`
- listens on an ephemeral loopback port
- opens the system browser to Xero login
- validates `state` on callback before exchanging the code
- stores tokens in `~/.config/xero/tokens.json` with `0600` permissions for MVP
- writes non-secret metadata to `~/.config/xero/session.json`
- prompts for a default tenant when multiple tenants are returned

## Refresh policy

- the CLI persists `generatedAt` on every token write
- refresh runs only when `now - generatedAt > 25m`
- non-interactive commands fail cleanly when refresh cannot recover the session
- interactive commands may re-authenticate in the browser if refresh fails and `--no-browser` is not set

## Troubleshooting

- missing client ID: set `XERO_AUTH_CLIENT_ID`
- callback timeout: verify the browser can reach `127.0.0.1`
- stale tenant: rerun `xero auth login` to discover and choose a valid tenant again
- token storage: inspect `~/.config/xero/tokens.json` permissions and rerun `xero doctor`
