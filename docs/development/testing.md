# Testing

## Local commands

```bash
go test ./...
go test ./test/auth ./test/commands ./test/output ./test/integration
go vet ./...
```

## Coverage focus

- auth state parsing and corrupt-state handling
- config precedence across flags, env, and persisted config
- profile resolution, encrypted token storage, and tenant metadata stored with profile tokens
- refresh gating around token `expiresAt` and the one-minute refresh buffer
- JSON contract stability for `--json` and `--quiet`
- invoice command integration across token refresh and direct Xero API invocation
- exact invoice and bill mutation method, path, headers, and JSON body mapping
- update type preflight and complete line-item replacement safeguards
- attachment collision, size, MIME, stream-length, and uncertain-outcome handling

## Credentials

Integration tests in this repo use local fakes. Real end-to-end validation still requires a Xero OAuth app and at least one tenant with invoice data.

For an opt-in sandbox run, build the CLI and follow [the write workflow checklist](../test/2026-07-10-invoice-bill-write-sandbox-checklist.md). Use a dedicated demo organisation, keep new transactions in `DRAFT`, record returned IDs, and change those drafts to `DELETED` when the run is complete. Never put profile tokens, tenant IDs, contact data, or live command output into committed fixtures.
