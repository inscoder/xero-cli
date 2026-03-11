---
title: feat: Add advanced filtering to xero invoices
type: feat
status: completed
date: 2026-03-11
---

# feat: Add advanced filtering to xero invoices

## Overview

Expand `xero invoices` so users can filter invoices more precisely without dropping to raw API calls. The command should support optimized list filtering by invoice ID and status, expose Xero's `where` clause for advanced cases, add customizable ordering with a CLI default of `UpdatedDateUTC DESC`, and return complete invoice data instead of the current small summary projection.

The goal is to keep the top-level terminal and JSON envelope behavior stable while widening the query surface and expanding each invoice record to match Xero's returned data much more closely.

## Problem Statement

The current `xero invoices` command only supports a narrow filter set: one `--status`, `--since`, `--page`, and client-side `--limit` in `internal/commands/invoices_list.go:15`. The Xero client only maps `Statuses`, `page`, and `If-Modified-Since` in `internal/xeroapi/client.go:99`, and it does not expose Xero's native `pageSize` parameter.

That leaves several common invoice workflows either unsupported or awkward:

- operators cannot fetch a specific set of invoices by ID
- users cannot pass multiple statuses in one command
- optimized Xero `where` filtering for `Type`, `Date`, `DueDate`, `AmountDue`, and exact contact matching is unavailable
- result ordering cannot be controlled, which makes scripting and paging less predictable
- the current invoice output only includes a few flattened fields, which is not enough for downstream automation or inspection

Because this command sits directly on top of Xero's Accounting API, the change is partly a CLI UX feature and partly an API-mapping feature. The plan needs to make those boundaries explicit so the command stays predictable for humans and safe for scripts.

## Proposed Solution

Add four filtering capabilities to `xero invoices` and widen the invoice output payload:

1. `--invoice-id` for one or more invoice IDs, mapped to Xero's optimized `IDs` query param.
2. `--status` as a multi-value filter, preserving the existing flag name for backward compatibility while mapping to Xero's `Statuses` query param.
3. `--where` as a raw pass-through for advanced Xero filtering, with documentation and examples focused on optimized fields.
4. `--order` for custom ordering, with a CLI default of `UpdatedDateUTC DESC` when the flag is omitted.
5. Return the full invoice data available from Xero's collection response in `--json` and `--quiet`, instead of reducing each invoice to a small hand-picked subset.

Recommended contract decisions for v1 of this feature:

- keep `--status` instead of introducing `--statuses`, but make it repeatable and comma-aware
- add `--invoice-id` as a repeatable and comma-aware flag
- keep existing `--since` and `--page` behavior, but replace local `--limit` with API-backed `--page-size`
- treat `--where` as an advanced escape hatch: pass the clause through unchanged instead of trying to parse the full Xero expression grammar locally
- validate `--order` syntax locally so obvious mistakes fail before the request is sent, while documenting Xero's optimized order fields
- require `--page-size` to be used with `--page`, matching Xero's documented API behavior
- keep human-readable output concise for terminal use, but make structured output include the full invoice record shape

### Suggested command examples

```bash
xero invoices --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --invoice-id 88192a99-cbc5-4a66-bf1a-2f9fea2d36d0
xero invoices --status AUTHORISED --status PAID
xero invoices --page 1 --page-size 250 --status AUTHORISED --status PAID
xero invoices --where 'Type=="ACCPAY" AND Status=="AUTHORISED"'
xero invoices --where 'Date>=DateTime(2026, 01, 01) AND Date<DateTime(2026, 02, 01)'
xero invoices --order 'Date ASC'
```

## Technical Approach

### Architecture

The implementation should stay within the existing split between command parsing and Xero API request construction:

- `internal/commands/invoices_list.go`: extend flags, normalize repeated and comma-separated inputs, apply lightweight validation, and pass a richer request object downstream
- `internal/xeroapi/client.go`: map new request fields to `IDs`, `Statuses`, `where`, `order`, and `pageSize` query params while preserving `If-Modified-Since` and `page`
- `internal/xeroapi/client.go`: decode and expose a much richer invoice payload instead of flattening to a few summary fields
- `test/commands/invoices_test.go`: verify CLI parsing, defaults, and validation behavior
- `test/xeroapi/client_test.go`: verify exact outbound query construction and response handling
- `internal/output/json.go` and any invoice-specific human formatter: keep the envelope stable while adapting record rendering to the richer invoice payload
- `docs/commands/invoices.md` and `README.md`: document the expanded flag surface and shell-safe examples

Recommended request model update in `internal/xeroapi/client.go`:

```go
// internal/xeroapi/client.go
type ListInvoicesRequest struct {
    TenantID   string
    InvoiceIDs []string
    Statuses   []string
    Since      string
    Where      string
    Order      string
    Page       int
    PageSize   int
}
```

Recommended invoice output model update:

- replace the current compact invoice projection with a struct that mirrors Xero's invoice collection payload far more closely
- preserve nested objects where possible instead of flattening everything into top-level strings
- include fields such as `Type`, `Date`, `DueDate`, `Status`, `LineAmountTypes`, `LineItems`, `SubTotal`, `TotalTax`, `Total`, `TotalDiscount`, `AmountDue`, `AmountPaid`, `AmountCredited`, `CurrencyCode`, `CurrencyRate`, `Reference`, `BrandingThemeID`, `Url`, `SentToContact`, `ExpectedPaymentDate`, `PlannedPaymentDate`, `HasAttachments`, `Payments`, `CreditNotes`, `Prepayments`, `Overpayments`, and contact details returned in the collection response
- retain normalized date/timestamp formatting where the CLI already converts Xero date wrappers into ISO-style output

Output strategy recommendation:

- `--json` and `--quiet` should emit the full normalized invoice object
- default human-readable output can remain a concise table or summary row set, with room to add a follow-up detailed mode later if needed

Representative structured output target:

```json
{
  "invoiceId": "e6b1f2bf-f9df-4738-8e1d-ef65e1bc1f04",
  "type": "ACCREC",
  "invoiceNumber": "INV-0001",
  "reference": "PO-123",
  "contact": {
    "contactId": "...",
    "name": "Apple"
  },
  "date": "2022-04-03",
  "dueDate": "2022-05-03",
  "status": "PAID",
  "subTotal": 579,
  "totalTax": 0,
  "total": 579,
  "amountDue": 0,
  "amountPaid": 579,
  "currencyCode": "HKD",
  "updatedAt": "2022-05-10T00:48:29Z",
  "hasAttachments": false,
  "lineItems": [],
  "payments": []
}
```

Recommended validation boundaries:

- validate that `--invoice-id` and `--status` do not contain empty elements after normalization
- normalize statuses to uppercase before sending them
- validate status values against the scoped set: `DRAFT`, `SUBMITTED`, `DELETED`, `AUTHORISED`, `PAID`, `VOIDED`
- validate `--order` against supported directions `ASC` and `DESC`
- validate only basic `--order` syntax locally, and document that Xero optimizes `InvoiceId`, `UpdatedDateUTC`, and `Date` for ordering
- validate that `--page-size` is a positive integer and requires `--page`
- do not attempt to parse or rewrite `--where`; instead, reject only obviously empty input and rely on Xero for clause-level validation

Recommended wire-format rules:

- join `InvoiceIDs` with commas into `IDs`
- join `Statuses` with commas into `Statuses`
- send `where` exactly once when present
- send `order` exactly once; if absent, inject `UpdatedDateUTC DESC`
- send `pageSize` only when `PageSize > 0`
- preserve current `If-Modified-Since` header behavior for `--since`
- stop trimming invoice results locally in the client once `pageSize` is supported remotely

### Implementation Phases

#### Phase 1: Request contract and validation

Deliverables:

- update `internal/xeroapi/client.go` request struct for multi-value and advanced filter fields
- extend `internal/commands/invoices_list.go` flag parsing for `--invoice-id`, repeatable `--status`, `--where`, `--order`, and `--page-size`
- add command-layer normalization helpers for comma-separated and repeated flag input
- add typed validation errors for malformed statuses, empty IDs, invalid order clauses, and `--page-size` misuse
- design the richer invoice output struct so it matches Xero data without breaking top-level output wrappers

Success criteria:

- command accepts the new flags without breaking existing `--status`, `--since`, and `--page` usage
- invalid inputs fail with `clierrors.KindValidation`
- request objects passed to the client are normalized and deterministic
- the response model is ready to carry full invoice payloads instead of the current narrow summary object

Estimated effort:

- 0.5-1 engineering day

#### Phase 2: Xero API mapping and tests

Deliverables:

- map `IDs`, `Statuses`, `where`, `order`, and `pageSize` in `internal/xeroapi/client.go`
- expand invoice decoding and normalization to cover the full Xero invoice payload used by the CLI
- expand `test/xeroapi/client_test.go` with query construction coverage for all new filters
- expand `test/commands/invoices_test.go` with command parsing and default-order coverage
- update `test/integration/xero_invoices_integration_test.go` so the fake integration path exercises at least one advanced filter combination

Success criteria:

- outbound requests match the intended Xero query params exactly
- `pageSize` maps directly to Xero instead of relying on local post-fetch trimming
- existing invoice listing behavior remains intact when no new flags are used
- JSON and quiet output include substantially all invoice fields returned by Xero instead of the current compact subset

Estimated effort:

- 0.5-1 engineering day

#### Phase 3: Documentation and polish

Deliverables:

- update `docs/commands/invoices.md` with full flag descriptions and real examples
- update `README.md` examples to reflect the richer invoice filter surface
- add notes about Xero optimization guidance, shell quoting, and paging caveats
- update output examples so they show the richer invoice schema
- verify `go test ./...` and focused command/client/integration test suites

Success criteria:

- a user can discover and correctly use the new filters from repo docs alone
- docs explain which filters are optimized and which behaviors are Xero-defined
- tests cover both happy path and validation failures
- docs make it clear that structured output now returns full invoice data

Estimated effort:

- 0.5 engineering day

## Alternative Approaches Considered

### 1. Add only `--where` and skip helper flags

Rejected because it would technically expose power but would make common use cases worse. Fetching by ID and multi-status queries are important enough to deserve first-class, discoverable flags.

### 2. Replace `--status` with a new `--statuses` flag

Rejected because it creates an unnecessary breaking change in a very new command surface. Keeping `--status` and making it multi-value preserves compatibility while aligning with Xero's `Statuses` parameter.

### 3. Parse and validate the full Xero `where` grammar locally

Rejected for this iteration because it would add a large parser/DSL surface to the CLI, increase maintenance cost, and still lag behind Xero semantics. A pass-through `--where` with focused docs is the better tradeoff.

### 4. Leave ordering entirely to Xero defaults

Rejected because the feature request explicitly needs customizable ordering and a default descending sort for better CLI ergonomics. The plan should make that override explicit and testable.

### 5. Keep the current compact invoice output shape

Rejected because the current payload drops too much invoice data for real use. Users should not need a second API call or code change just to access fields Xero already returns.

## System-Wide Impact

### Interaction Graph

`xero invoices` flag parsing in `internal/commands/invoices_list.go:15` builds `xeroapi.ListInvoicesRequest`, which flows through runtime auth and tenant resolution before calling `rt.Xero.ListInvoices(...)`. That in turn builds the outbound HTTP request in `internal/xeroapi/client.go:99`, sends `GET /api.xro/2.0/Invoices`, includes `pageSize` when requested, decodes the payload, normalizes invoice fields, and returns richer invoice records to the shared output writer.

Adding filters changes the request object and the URL-building branch. Expanding invoice records also changes the structured `data` payload, but it should not change auth, tenant selection, or the top-level JSON envelope generation.

### Error & Failure Propagation

Command-layer validation failures should continue to return `clierrors.KindValidation` before any network activity. Xero-side clause or query errors will still come back through the HTTP client and currently map to `KindRateLimit`, `KindAuthRequired`, or `KindXeroAPI` in `internal/xeroapi/client.go:138`.

This feature should preserve that split:

- malformed CLI input -> typed validation error
- malformed but syntactically pass-through `where` accepted by CLI -> Xero API error surfaced cleanly
- auth and tenant failures -> unchanged current behavior

### State Lifecycle Risks

This feature does not add new persisted state, models, migrations, or background processes. The main risk is user confusion rather than data corruption:

- `--page-size` must follow Xero's rule that it is used together with `--page`
- changing the default order to descending can change perceived result stability across pages
- raw `--where` quoting errors can make the command appear flaky unless docs include shell-safe examples
- widening invoice output may surprise scripts that assumed the old compact invoice schema

### API Surface Parity

The changed behavior is localized to the `xero invoices` command and the shared Xero invoice client. Related surfaces that must stay in sync:

- CLI help text in `internal/commands/invoices_list.go:17`
- request struct and URL mapping in `internal/xeroapi/client.go:35`
- command docs in `docs/commands/invoices.md:1`
- examples in `README.md:5`
- fake-based tests in `test/commands/invoices_test.go:56` and `test/xeroapi/client_test.go:17`

### Integration Test Scenarios

Cross-layer scenarios worth covering:

1. `xero invoices --status AUTHORISED --status PAID --json` builds `Statuses=AUTHORISED,PAID`, preserves tenant resolution, and emits the existing JSON envelope.
2. `xero invoices --invoice-id <id1> --invoice-id <id2> --order 'UpdatedDateUTC DESC'` builds `IDs` and `order` together without changing response normalization.
3. `xero invoices --where 'Type=="ACCPAY" AND AmountDue>=5000' --page 2 --page-size 100` preserves raw clause encoding and maps paging controls directly to Xero.
4. `xero invoices --order 'UpdatedDateUTC backwards'` fails locally with a validation error and performs no network request.
5. Auth refresh or tenant lookup failure still behaves exactly as it does today even when advanced filter flags are present.
6. `xero invoices --json` returns nested and numeric invoice fields such as contact data, totals, references, payments, and attachment indicators rather than only `invoiceId`, `invoiceNumber`, `contactName`, `status`, `total`, `currency`, `dueDate`, and `updatedAt`.

## Acceptance Criteria

### Functional Requirements

- [x] `xero invoices` supports one or many `--invoice-id` values and maps them to Xero's `IDs` query parameter.
- [x] `xero invoices` supports one or many `--status` values while remaining backward compatible with the existing singular usage.
- [x] `xero invoices` supports a raw `--where` clause and passes it through without rewriting.
- [x] `xero invoices` supports `--order`, and when omitted the client sends `UpdatedDateUTC DESC`.
- [x] `xero invoices` supports `--page-size`, maps it to Xero's `pageSize` query parameter, and rejects it when `--page` is absent.
- [x] `xero invoices --json` and `xero invoices --quiet` return full invoice data from the Xero collection response instead of the current compact subset.
- [x] Existing flags `--since`, `--page`, `--tenant`, `--json`, `--quiet`, and `--no-browser` continue to work.
- [x] Validation errors for malformed IDs, empty multi-value input, unknown statuses, malformed order syntax, and invalid `--page-size` usage are clear and typed.

### Non-Functional Requirements

- [x] The top-level JSON envelope shape is unchanged, even though each invoice record becomes richer.
- [x] No new persisted config or token state is introduced.
- [x] Query construction remains deterministic and URL encoding is covered by tests.
- [x] Paging behavior follows Xero's remote API semantics instead of client-side trimming.
- [x] Date and timestamp normalization remains consistent across the expanded invoice fields.
- [x] Documentation includes shell-safe examples for `bash` and `zsh` style quoting.

### Quality Gates

- [x] Update `test/commands/invoices_test.go` for parsing, defaults, and validation.
- [x] Update `test/xeroapi/client_test.go` for request mapping and error handling.
- [x] Update `test/integration/xero_invoices_integration_test.go` for at least one advanced filter path.
- [x] Add or update JSON output assertions so full invoice payload fields are covered.
- [x] Update `docs/commands/invoices.md` and `README.md` to match shipped behavior.
- [x] Run `go test ./...` before merging.

## Success Metrics

- advanced filters can be exercised entirely from the CLI without manual API URL building
- existing invoice command tests continue to pass with only intentional invoice payload expansion
- new tests prove exact mapping for `IDs`, `Statuses`, `where`, `order`, `page`, and `pageSize`
- docs provide enough examples that a developer can copy a working command for each new filter class
- structured output exposes enough invoice detail that follow-up API calls are no longer needed for common inspection use cases

## Dependencies & Prerequisites

- Xero Accounting API invoice filter semantics from `https://developer.xero.com/documentation/api/accounting/invoices`
- existing command/runtime wiring in `internal/commands/invoices_list.go:15`
- existing invoice client in `internal/xeroapi/client.go:99`
- existing fake-based command/client test structure in `test/commands/invoices_test.go:56` and `test/xeroapi/client_test.go:17`

## Risk Analysis & Mitigation

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Raw `where` syntax is confusing to shell users | High support friction | Add multiple quoted examples to `docs/commands/invoices.md` and keep CLI pass-through behavior simple |
| Custom default order changes user expectations | Medium | Document the descending default clearly in help text and docs |
| Xero optimization rules are broader or stricter than expected | Medium | Prefer helper flags for optimized list filters and keep `where` pass-through with strong docs |
| Over-validating `where` locally causes false negatives | Medium | Do not implement a local `where` parser in this iteration |
| Under-validating `order` creates avoidable API failures | Medium | Validate order shape and direction locally, and document Xero's optimized order fields prominently |
| `--page-size` is used without `--page` | Low | Reject the input locally and explain the required pairing in help text and docs |
| Expanded invoice payload breaks scripts expecting the old compact schema | Medium | Call out the change in docs, add fixture coverage, and preserve the top-level envelope |

## Resource Requirements

- 1 engineer familiar with Go, Cobra, and the current Xero client wiring
- approximately 1.5-2.5 engineering days including test and docs updates
- access to current fake test harness; optional real-tenant verification after local completion

## Future Considerations

- add first-class `--contact-id` and `--contact-name` flags if exact contact filtering becomes common enough to deserve helpers
- consider `--invoice-number` if users frequently fetch a known set of invoice numbers
- consider exposing `summaryOnly` later if users hit response-size or paging limits
- revisit stable secondary ordering if Xero's `order` syntax for multi-field ordering is confirmed and worth enforcing

## Documentation Plan

Update these docs as part of the same change:

- `docs/commands/invoices.md`: full flag reference, examples, and quoting guidance
- `README.md`: refreshed examples for multi-status, invoice IDs, `where`, `order`, and richer JSON output
- inline command help in `internal/commands/invoices_list.go:17`: concise descriptions for the new flags

Recommended example snippets to include in docs:

```bash
# docs/commands/invoices.md
xero invoices --status AUTHORISED,PAID
xero invoices --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734,88192a99-cbc5-4a66-bf1a-2f9fea2d36d0
xero invoices --page 1 --page-size 250 --status AUTHORISED
xero invoices --where 'Contact.Name=="ABC limited" AND DueDate<DateTime(2026, 04, 01)'
xero invoices --order 'UpdatedDateUTC DESC'

# JSON output example should show more than the current compact subset
xero invoices --json
```

## Sources & References

### Internal References

- existing command implementation: `internal/commands/invoices_list.go:15`
- current invoice request model and mapping: `internal/xeroapi/client.go:35`
- current outbound request behavior: `internal/xeroapi/client.go:99`
- current client request test coverage: `test/xeroapi/client_test.go:17`
- current command JSON contract coverage: `test/commands/invoices_test.go:56`
- current invoice command docs: `docs/commands/invoices.md:1`
- top-level CLI usage examples: `README.md:5`
- testing expectations for command and integration coverage: `docs/development/testing.md:1`
- auth and tenant behavior that must remain unchanged: `docs/auth.md:1`
- original invoice command MVP plan: `docs/plans/2026-03-10-feat-xero-cli-browser-auth-invoices-plan.md:101`

### External References

- Xero Accounting API invoices: `https://developer.xero.com/documentation/api/accounting/invoices`
- Xero optimized `where` guidance: `https://developer.xero.com/documentation/api/accounting/invoices#optimised-use-of-the-where-filter`

### AI-Assisted Research Notes

- local repo research identified the existing command, client, tests, and docs touch points
- browser-assisted doc capture was used because the Xero docs page requires JavaScript for full content
- spec-flow analysis highlighted the need to resolve helper-flag compatibility, validation scope, and paging/order edge cases before implementation
