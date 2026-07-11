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
- contact list filtering and normalization, strict create/update input modes, archive confirmation, and uncertain-outcome handling

## Credentials

Integration tests in this repo use local fakes. Real end-to-end validation still requires a Xero OAuth app and a dedicated demo tenant suitable for synthetic invoice and contact records.

For an opt-in sandbox run, build the CLI and follow the [invoice and bill write workflow checklist](../test/2026-07-10-invoice-bill-write-sandbox-checklist.md) or the [contact workflow checklist](../test/2026-07-11-contacts-sandbox-checklist.md). Every live contact API command must explicitly pass `-p demo --no-browser`; archive the synthetic contact at the end, and never put profile tokens, tenant IDs, ContactIDs, contact data, or live command output into committed fixtures.
