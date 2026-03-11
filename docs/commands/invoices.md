# `xero invoices`

## Usage

```bash
xero auth login
xero invoices --status AUTHORISED,PAID --page 1 --page-size 100
xero invoices --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --order "UpdatedDateUTC DESC"
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices --where 'Type=="ACCPAY" AND AmountDue>=5000'
xero invoices --tenant <tenant-id> --json
```

## Flags

- `--tenant <tenant-id>`: override the saved default tenant
- `--invoice-id <uuid[,uuid...]>`: filter by one or more invoice IDs; repeatable and comma-separated
- `--status <status[,status...]>`: filter by one or more statuses; repeatable and comma-separated
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
- invoice `url` in list output is not the customer-facing online invoice URL; use `xero invoices online-url` for that workflow

## `xero invoices pdf`

### Usage

```bash
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output -
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf --json
```

### Flags

- `--invoice-id <uuid>`: required invoice ID to resolve through Xero's PDF endpoint
- `-o, --output <path|->`: required output destination; use `-` to stream raw PDF bytes to stdout
- `--tenant <tenant-id>`: override the saved default tenant
- `--json`: emit the JSON envelope with saved-file metadata only
- `--quiet`: emit raw saved-file metadata only
- `--no-browser`: fail instead of opening a browser when auth is required

### Notes

- this command calls Xero's invoice PDF retrieval contract with `Accept: application/pdf`
- file output is explicit in v1; the command does not auto-generate a filename
- `--output -` streams raw PDF bytes to stdout and cannot be combined with `--json` or `--quiet`
- the command refuses to dump raw PDF bytes to an interactive terminal; use a file path or pipe stdout

### JSON example

```json
{
  "ok": true,
  "data": {
    "invoiceId": "220ddca8-3144-4085-9a88-2d72c5133734",
    "contentType": "application/pdf",
    "bytes": 48213,
    "output": "file",
    "savedTo": "invoice.pdf",
    "streamed": false
  },
  "summary": "invoice PDF saved",
  "breadcrumbs": [
    {
      "action": "show",
      "cmd": "xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf --tenant <tenant-id> --json"
    }
  ]
}
```

## `xero invoices online-url`

### Usage

```bash
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --json
```

### Flags

- `--invoice-id <uuid>`: required invoice ID to resolve through Xero's dedicated online-invoice endpoint
- `--tenant <tenant-id>`: override the saved default tenant
- `--json`: emit the JSON envelope
- `--quiet`: emit raw `data` only
- `--no-browser`: fail instead of opening a browser when auth is required

### Notes

- this command calls `GET /Invoices/{InvoiceID}/OnlineInvoice`
- when a URL exists, the default human output prints the URL only
- when Xero returns no online invoice URL, the command exits successfully and explains that no URL is available yet

### JSON example

```json
{
  "ok": true,
  "data": {
    "invoiceId": "220ddca8-3144-4085-9a88-2d72c5133734",
    "onlineInvoiceUrl": "https://in.xero.com/abc",
    "available": true
  },
  "summary": "online invoice URL available",
  "breadcrumbs": [
    {
      "action": "show",
      "cmd": "xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --tenant <tenant-id> --json"
    }
  ]
}
```

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
