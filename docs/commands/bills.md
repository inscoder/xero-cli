# `xero bills`

## Usage

```bash
xero profile add my-company --client-id YOUR_CLIENT_ID
xero login -p my-company
xero bills --status AUTHORISED --page 1 --page-size 100
xero bills --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --order "UpdatedDateUTC DESC"
xero bills --where 'AmountDue>=5000'
xero bills -p my-company --json
```

## Flags

- `-p, --profile <name>`: use a named Xero profile
- `--invoice-id <uuid[,uuid...]>`: filter by one or more Xero invoice IDs; repeatable and comma-separated
- `--status <status[,status...]>`: filter by one or more statuses; repeatable and comma-separated
- `--since <YYYY-MM-DD>`: filter recently updated bills
- `--where <clause>`: advanced Xero `where` clause for optimized fields such as `Date`, `DueDate`, `AmountDue`, and exact contact matching
- `--order "<Field> <ASC|DESC>"`: custom ordering, defaults to `UpdatedDateUTC DESC`
- `--page <n>`: explicit page number, defaults to `1`
- `--page-size <n>`: API page size, using page `1` unless `--page` is provided
- `--json`: emit the JSON envelope
- `--quiet`: emit raw `data` only
- `--no-browser`: fail instead of opening a browser when auth is required

## Notes

- `xero bills` lists purchase bills by applying Xero invoice `Type=="ACCPAY"` automatically.
- Without paging flags, the CLI requests page `1` from Xero to avoid unbounded bill retrieval.
- The command calls Xero's `GET /Invoices` endpoint because Xero models purchase bills as invoices with `Type` set to `ACCPAY`.
- Use `xero invoices` for sales invoices (`Type=="ACCREC"`).
- `--where` is combined with the command's bill type and passed to Xero, so quote it in your shell. Do not include `Type` in `--where`.
- `xero bills` supports listing only. Invoice actions such as `approve`, `pdf`, and `online-url` remain under `xero invoices`.

## JSON Example

```json
{
  "ok": true,
  "data": [
    {
      "invoiceId": "e6b1f2bf-f9df-4738-8e1d-ef65e1bc1f04",
      "type": "ACCPAY",
      "invoiceNumber": "BILL-0001",
      "status": "AUTHORISED",
      "total": 579,
      "amountDue": 579,
      "currencyCode": "HKD"
    }
  ],
  "summary": "1 bill",
  "breadcrumbs": [
    {
      "action": "show",
      "cmd": "xero bills --json"
    }
  ]
}
```
