# `xero invoices`

## Usage

```bash
xero auth login
xero invoices --status AUTHORISED,PAID --page 1 --page-size 100
xero invoices --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --order "UpdatedDateUTC DESC"
xero invoices --where 'Type=="ACCPAY" AND AmountDue>=5000'
xero invoices --tenant <tenant-id> --json
```

## Flags

- `--tenant <tenant-id>`: override the saved default tenant
- `--invoice-id <uuid[,uuid...]>`: filter by one or more invoice IDs; repeatable and comma-separated
- `--status <status[,status...]>`: filter by one or more statuses; repeatable and comma-separated
- `--contact <name-or-id>`: filter by contact
- `--since <YYYY-MM-DD>`: filter recent invoices
- `--where <clause>`: advanced Xero `where` clause for optimized fields such as `Type`, `Date`, `DueDate`, `AmountDue`, and exact contact matching
- `--order "<Field> <ASC|DESC>"`: custom ordering, defaults to `UpdatedDateUTC DESC`
- `--page <n>`: explicit page number
- `--page-size <n>`: API page size; requires `--page`
- `--json`: emit the JSON envelope
- `--quiet`: emit raw `data` only
- `--no-browser`: fail instead of opening a browser when auth is required

## Notes

- `--page-size` maps directly to Xero's `pageSize` query parameter and is only valid when `--page` is present
- `--where` is passed through directly to Xero, so quote it in your shell
- `--json` and `--quiet` now return full invoice records rather than a compact invoice summary

## JSON example

```json
{
  "ok": true,
  "data": [
    {
      "invoiceId": "e6b1f2bf-f9df-4738-8e1d-ef65e1bc1f04",
      "type": "ACCREC",
      "invoiceNumber": "INV-0001",
      "reference": "PO-123",
      "contact": {
        "contactId": "contact-1",
        "name": "Apple"
      },
      "date": "2022-04-03",
      "status": "AUTHORISED",
      "lineItems": [],
      "subTotal": 579,
      "totalTax": 0,
      "total": 579,
      "amountDue": 0,
      "amountPaid": 579,
      "currencyCode": "HKD",
      "dueDate": "2022-05-03",
      "updatedAt": "2022-05-10T00:48:29Z",
      "payments": [],
      "creditNotes": [],
      "prepayments": [],
      "overpayments": [],
      "contactName": "Apple",
      "currency": "HKD"
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
