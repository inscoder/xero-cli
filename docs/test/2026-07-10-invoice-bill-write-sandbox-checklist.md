# Invoice and bill write sandbox checklist

Use this only with a dedicated Xero demo organisation. Replace the sample UUIDs and account codes with values valid for that tenant.

## Prepare

- Build the binary: `go build -o /tmp/xero ./cmd/xero`.
- Confirm the target profile: `/tmp/xero profile list --json`.
- Confirm read access without browser fallback: `/tmp/xero invoices -p demo --no-browser --page-size 1 --json`.
- Use unique references and caller-supplied idempotency keys for the run.

## Exercise

- Create one `DRAFT` invoice and record its `invoiceId` and `idempotencyKey`.
- Create one `DRAFT` bill and record its `invoiceId` and `idempotencyKey`.
- Update a scalar field on each document and verify the result by ID.
- Upload one small regular attachment to each document.
- Repeat without `--overwrite` and confirm the collision is rejected before mutation.
- Repeat with `--overwrite` and confirm `overwritten: true`.
- For the sales invoice, upload a new filename with `--include-online` and confirm `includeOnline: true`.
- Confirm `--include-online` is absent from bill help.

## Clean up

- Update each `DRAFT` document to `{ "status": "DELETED" }`.
- Read each ID and confirm its final status.
- Remove local temporary inputs and attachments.
- Do not commit tenant IDs, contact IDs, live invoice IDs, access tokens, or response payloads.

If a mutation returns `MutationUncertain`, run the reported recovery command before retrying. Reuse the same idempotency key only for an exact retry of the same operation and payload.
