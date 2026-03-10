# Testing

## Local commands

```bash
go test ./...
go test ./test/auth ./test/commands ./test/output ./test/integration
```

## Coverage focus

- auth state parsing and corrupt-state handling
- config precedence across flags, env, and persisted config
- tenant resolution and revoked-tenant handling
- refresh gating above and below the 25-minute threshold
- JSON contract stability for `--json` and `--quiet`
- invoice command integration across token refresh and direct Xero API invocation

## Credentials

Integration tests in this repo use local fakes. Real end-to-end validation still requires a Xero OAuth app and at least one tenant with invoice data.
