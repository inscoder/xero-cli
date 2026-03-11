# Invoice Advanced Filtering Test Matrix

Date: 2026-03-11
Branch: `feat/invoice-advanced-filtering`
PR: `https://github.com/inscoder/xero-cli/pull/1`

## Test Commands Run

```bash
go test ./test/xeroapi ./test/commands ./test/output ./test/integration
go test ./...
```

## Coverage by Feature

| Feature | Coverage | Tests |
| --- | --- | --- |
| Multi-invoice ID filter via `--invoice-id` | Unit | `test/commands/invoices_test.go:83`, `test/xeroapi/client_test.go:17` |
| Multi-status filter via repeatable/comma-aware `--status` | Unit + Integration | `test/commands/invoices_test.go:83`, `test/xeroapi/client_test.go:17`, `test/integration/xero_invoices_integration_test.go:21` |
| Raw `--where` passthrough | Unit | `test/commands/invoices_test.go:83`, `test/xeroapi/client_test.go:17` |
| Custom `--order` mapping and default descending order on live command path | Unit + Integration | `test/commands/invoices_test.go:83`, `test/xeroapi/client_test.go:17`, `test/integration/xero_invoices_integration_test.go:21` |
| `--page-size` mapping to Xero `pageSize` | Unit + Integration | `test/commands/invoices_test.go:83`, `test/xeroapi/client_test.go:17`, `test/integration/xero_invoices_integration_test.go:21` |
| `--page-size` requires `--page` | Unit | `test/commands/invoices_test.go:121` |
| Unknown status validation | Unit | `test/commands/invoices_test.go:142` |
| Removed `--contact` flag is rejected | Unit | `test/commands/invoices_test.go:160` |
| Rich invoice payload decoding from Xero response | Unit | `test/xeroapi/client_test.go:17` |
| Rich JSON envelope output | Unit | `test/output/json_contract_test.go:11`, `test/commands/invoices_test.go:57` |
| Rich quiet/raw JSON output | Unit | `test/output/json_contract_test.go:53` |
| End-to-end auth refresh + tenant resolution + invoice query | Integration | `test/integration/xero_invoices_integration_test.go:21` |
| Existing typed auth failure behavior | Unit regression | `test/commands/invoices_test.go:160` |
| Existing Xero rate limit error mapping | Unit regression | `test/xeroapi/client_test.go:107` |

## What Each Test Proves

- `test/commands/invoices_test.go:57` proves `xero invoices --json` still emits the standard envelope and now includes richer invoice fields.
- `test/commands/invoices_test.go:83` proves command-level parsing and normalization for invoice IDs, statuses, `where`, `order`, `page`, `page-size`, and `since`.
- `test/commands/invoices_test.go:121` proves invalid paging combinations fail before the client is called.
- `test/commands/invoices_test.go:142` proves invalid statuses fail with a typed validation error.
- `test/commands/invoices_test.go:160` proves the removed `--contact` flag is rejected instead of silently mapping to a search term.
- `test/xeroapi/client_test.go:17` proves the client sends the correct Xero query params and normalizes nested invoice data, dates, timestamps, payments, and allocations.
- `test/output/json_contract_test.go:11` proves the top-level JSON envelope stays stable while invoice objects become richer.
- `test/output/json_contract_test.go:53` proves `--quiet` returns raw full invoice data without the envelope.
- `test/integration/xero_invoices_integration_test.go:21` proves auth refresh, tenant resolution, advanced invoice params, and rich JSON output work together on the real command path.

## Current Gaps

These are not covered by dedicated tests yet:

- invalid `--invoice-id` format rejection
- malformed `--order` rejection
- empty `--where` rejection
- human-readable table formatting for the richer invoice model

The shipped change is covered for the main happy paths and key validation paths, but the items above would be good follow-up unit tests.
