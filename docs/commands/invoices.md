# `xero invoices`

## Usage

```bash
xero profile add my-company --client-id YOUR_CLIENT_ID
xero login -p my-company
xero invoices --status AUTHORISED,PAID --page 1 --page-size 100
xero invoices --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --order "UpdatedDateUTC DESC"
xero invoices approve --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices --where 'Type=="ACCPAY" AND AmountDue>=5000'
xero invoices -p my-company --json
```

## Flags

- `-p, --profile <name>`: use a named Xero profile
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

- The selected Xero tenant is stored with the active profile token during `xero login`.
- There is no temporary tenant override. Use `-p, --profile` to select a different logged-in organisation.
- `--page-size` maps directly to Xero's `pageSize` query parameter and is only valid when `--page` is present.
- `--where` is passed through directly to Xero, so quote it in your shell.
- Invoice `url` in list output is not the customer-facing online invoice URL; use `xero invoices online-url` for that workflow.

## `xero invoices approve`

```bash
xero invoices approve --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices approve --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --json
```

- `--invoice-id <uuid>`: required invoice ID to approve
- required Xero scopes are `accounting.transactions` for legacy apps or `accounting.invoices` for granular-scope apps
- if the network outcome is unclear after dispatch, verify final state with `xero invoices --invoice-id <uuid> --json`

## `xero invoices pdf`

```bash
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output -
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf --json
```

- `--invoice-id <uuid>`: required invoice ID to resolve through Xero's PDF endpoint
- `-o, --output <path|->`: required output destination; use `-` to stream raw PDF bytes to stdout
- `--output -` streams raw PDF bytes to stdout and cannot be combined with `--json` or `--quiet`
- the command refuses to dump raw PDF bytes to an interactive terminal; use a file path or pipe stdout

## `xero invoices online-url`

```bash
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --json
```

- `--invoice-id <uuid>`: required invoice ID to resolve through Xero's dedicated online-invoice endpoint
- this command calls `GET /Invoices/{InvoiceID}/OnlineInvoice`
- when a URL exists, the default human output prints the URL only
- when Xero returns no online invoice URL, the command exits successfully and explains that no URL is available yet

## JSON Example

```json
{
  "ok": true,
  "data": [
    {
      "invoiceId": "e6b1f2bf-f9df-4738-8e1d-ef65e1bc1f04",
      "type": "ACCREC",
      "invoiceNumber": "INV-0001",
      "status": "AUTHORISED",
      "total": 579,
      "amountDue": 0,
      "currencyCode": "HKD"
    }
  ],
  "summary": "1 invoice",
  "breadcrumbs": [
    {
      "action": "show",
      "cmd": "xero invoices --json"
    }
  ]
}
```
