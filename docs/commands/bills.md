# `xero bills`

## Usage

```bash
xero profile add my-company --client-id YOUR_CLIENT_ID
xero login -p my-company
xero bills --status AUTHORISED --page 1 --page-size 100
xero bills --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --order "UpdatedDateUTC DESC"
xero bills create --file docs/examples/bill-create.json
xero bills update --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --file bill-update.json
xero bills attachments upload --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --file receipt.pdf
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
- Invoice actions such as `approve`, `pdf`, and `online-url` remain under `xero invoices`.

## `xero bills create`

```bash
xero bills create --file docs/examples/bill-create.json
```

- `--file <path|->` is required and contains exactly one purchase-bill object.
- `--idempotency-key <value>` optionally supplies a 1-128 byte key to reuse only for an exact retry. The CLI generates a random key otherwise.
- the command injects `Type: ACCPAY`; `type` and `invoiceId` are rejected as input fields.
- `contactId` and a non-empty `lineItems` array are required.
- `plannedPaymentDate` is supported for bills; invoice-only fields such as `sentToContact`, `expectedPaymentDate`, and line discounts are rejected locally.
- the strict JSON reader rejects inputs over 1 MiB, arrays, wrappers, unknown or duplicate fields, `null`, a BOM, and trailing data.

See [the create example](../examples/bill-create.json).

## `xero bills update`

```bash
xero bills update \
  --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 \
  --file bill-update.json
```

- `--invoice-id <uuid>` and `--file <path|->` are required.
- input is a partial object with at least one supported field.
- the command reads the target first and refuses to mutate it unless its Xero type is `ACCPAY`.
- when `lineItems` is present it is the complete desired set and requires `--replace-line-items`. Existing line IDs omitted from that array are removed.
- `--idempotency-key` follows the create contract.

## `xero bills attachments upload`

```bash
xero bills attachments upload \
  --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 \
  --file receipt.pdf

xero bills attachments upload \
  --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 \
  --file corrected-receipt.pdf \
  --filename receipt.pdf \
  --overwrite
```

- the file must be regular, non-empty, and no larger than 10,000,000 bytes; stdin is not supported.
- `--filename` and `--content-type` override the default remote name and detected MIME type.
- the command verifies the target is an `ACCPAY` bill and checks attachment collisions before mutation.
- a collision requires `--overwrite`; `--overwrite` is rejected when the remote name does not already exist.
- Xero permits at most 10 attachments per document.
- `--include-online` is intentionally unavailable for bills.

Write commands require `accounting.invoices`; attachment upload also requires `accounting.attachments`. Ambiguous post-dispatch outcomes use `MutationUncertain` (exit code 20) and include a safe recovery command and the effective idempotency key.

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
