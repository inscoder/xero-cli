# `xero invoices`

## Usage

```bash
xero auth login
xero invoices --status AUTHORISED --limit 20
xero invoices --tenant <tenant-id> --json
```

## Flags

- `--tenant <tenant-id>`: override the saved default tenant
- `--status <status>`: filter invoice status
- `--contact <name-or-id>`: filter by contact
- `--since <YYYY-MM-DD>`: filter recent invoices
- `--page <n>`: explicit page number
- `--limit <n>`: page size
- `--json`: emit the JSON envelope
- `--quiet`: emit raw `data` only
- `--no-browser`: fail instead of opening a browser when auth is required

## JSON example

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
  "summary": "1 invoice",
  "breadcrumbs": [
    {
      "action": "show",
      "cmd": "xero invoices --tenant <tenant-id> --json"
    }
  ]
}
```
