# `xero invoices`

## Usage

```bash
xero profile add my-company --client-id YOUR_CLIENT_ID
xero login -p my-company
xero invoices --status AUTHORISED,PAID --page 1 --page-size 100
xero invoices --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --order "UpdatedDateUTC DESC"
xero invoices create --file docs/examples/invoice-create.json
xero invoices update --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --file invoice-update.json
xero invoices attachments upload --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --file receipt.pdf
xero invoices approve --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices --where 'AmountDue>=5000'
xero invoices -p my-company --json
```

## Flags

- `-p, --profile <name>`: use a named Xero profile
- `--invoice-id <uuid[,uuid...]>`: filter by one or more invoice IDs; repeatable and comma-separated
- `--status <status[,status...]>`: filter by one or more statuses; repeatable and comma-separated
- `--since <YYYY-MM-DD>`: filter recent invoices
- `--where <clause>`: advanced Xero `where` clause for optimized fields such as `Date`, `DueDate`, `AmountDue`, and exact contact matching
- `--order "<Field> <ASC|DESC>"`: custom ordering, defaults to `UpdatedDateUTC DESC`
- `--page <n>`: explicit page number, defaults to `1`
- `--page-size <n>`: API page size, using page `1` unless `--page` is provided
- `--json`: emit the JSON envelope
- `--quiet`: emit raw `data` only
- `--no-browser`: fail instead of opening a browser when auth is required

## Notes

- The selected Xero tenant is stored with the active profile token during `xero login`.
- There is no temporary tenant override. Use `-p, --profile` to select a different logged-in organisation.
- `--page-size` maps directly to Xero's `pageSize` query parameter and uses the default first page unless `--page` is present.
- Without paging flags, the CLI requests page `1` from Xero to avoid unbounded invoice retrieval.
- `xero invoices` lists sales invoices (`Type=="ACCREC"`). Use `xero bills` for purchase bills (`Type=="ACCPAY"`).
- `--where` is combined with the command's invoice type and passed to Xero, so quote it in your shell. Do not include `Type` in `--where`.
- Invoice `url` in list output is not the customer-facing online invoice URL; use `xero invoices online-url` for that workflow.

## `xero invoices create`

```bash
xero invoices create --file docs/examples/invoice-create.json
printf '%s\n' '{"contactId":"eaa28f49-6028-4b6e-bb12-d8f6278073fc","lineItems":[{"description":"Consulting","quantity":1,"unitAmount":500,"accountCode":"200"}]}' \
  | xero invoices create --file - --json
```

- `--file <path|->` is required and contains exactly one sales-invoice object.
- `--idempotency-key <value>` optionally supplies the 1-128 byte key to reuse only for an exact retry. The CLI generates a random key otherwise.
- the command injects `Type: ACCREC`; `type` and `invoiceId` are rejected as input fields.
- `contactId` and a non-empty `lineItems` array are required. Create defaults to Xero's draft behavior when `status` is omitted.
- JSON is limited to 1 MiB and rejects arrays, wrappers, unknown or duplicate fields, `null`, a BOM, and trailing data before authentication or network access.

See [the create example](../examples/invoice-create.json).

## `xero invoices update`

```bash
xero invoices update \
  --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 \
  --file invoice-update.json

xero invoices update \
  --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 \
  --file docs/examples/invoice-update-lines.json \
  --replace-line-items
```

- `--invoice-id <uuid>` and `--file <path|->` are required.
- input is a partial object, but it must contain at least one supported field.
- the command reads the target first and refuses to mutate it unless its Xero type is `ACCREC`.
- `lineItems` is a complete replacement, never a patch. It is accepted only with `--replace-line-items`; passing that flag without `lineItems` is also rejected.
- preserve existing lines by including their `lineItemId`. Existing IDs omitted from the submitted array are removed. The result reports `lineItemsReplaced` and `removedLineItemCount`.
- `--idempotency-key` follows the create contract.

## `xero invoices attachments upload`

```bash
xero invoices attachments upload \
  --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 \
  --file receipt.pdf \
  --include-online

xero invoices attachments upload \
  --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 \
  --file corrected-receipt.pdf \
  --filename receipt.pdf \
  --overwrite
```

- `--invoice-id <uuid>` and `--file <path>` are required; attachment stdin is not supported.
- `--filename` overrides the source basename and `--content-type` overrides deterministic MIME detection.
- the file must be regular, non-empty, and no larger than 10,000,000 bytes. Xero permits at most 10 attachments per document.
- the command verifies the target is an `ACCREC` invoice and checks its attachment names before upload.
- a collision requires `--overwrite`; `--overwrite` is rejected when the name does not already exist.
- `--include-online` applies only to a new sales-invoice attachment and cannot be combined with `--overwrite`.
- `--idempotency-key` follows the create contract.

## Mutation outcomes

Known Xero validation failures include nested Xero messages in structured error output. If a connection failure or ambiguous server response occurs after dispatch, the command exits with `MutationUncertain` (exit code 20), sets `mayHaveSucceeded`, and includes the resource identifiers, effective idempotency key, and a read-only recovery command. Verify the document before deciding whether to retry with the same key.

Write commands require `accounting.invoices`; attachment upload also requires `accounting.attachments`. Re-run `xero login -p <profile>` after changing scopes.

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
