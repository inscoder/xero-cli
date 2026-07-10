---
title: "feat: Add invoice and bill write workflows"
type: feat
status: completed
date: 2026-07-10
---

# feat: Add invoice and bill write workflows

## Overview

Add first-class commands for creating and updating Xero sales invoices and purchase bills, plus uploading or intentionally replacing attachments on either resource.

The implementation must preserve the CLI's existing namespace rule:

- xero invoices owns Xero Type ACCREC.
- xero bills owns Xero Type ACCPAY.
- Both resources use Xero's Accounting API Invoices endpoints.

This feature expands the current single-purpose invoice approval write path into a general accounting-write surface. Because a lost response, wrong resource type, omitted line item, or duplicate attachment filename can change remote financial records, the command contract must make destructive behavior and uncertain outcomes explicit.

## Problem Statement

The CLI can list invoices and bills and can approve, download, or resolve URLs for sales invoices, but it cannot:

- create a sales invoice or purchase bill;
- update invoice or bill fields;
- replace the full line-item set intentionally;
- upload supporting documents to an invoice or bill; or
- safely distinguish a new attachment from a same-name replacement.

Users currently have to construct raw HTTP requests or switch to another tool. That bypasses the CLI's profile-based tenant selection, token refresh, typed errors, stable JSON output, and invoice-versus-bill namespace protections.

## Goals

- Provide parallel create, update, and attachment-upload workflows under both invoices and bills.
- Keep Xero Type owned by the command namespace rather than user input.
- Accept complex invoice data through a strict, documented JSON input contract.
- Protect direct-ID mutations with a read-before-write Type check.
- Make Xero's destructive line-item replacement semantics explicit.
- Make attachment collisions explicit and require an overwrite opt-in.
- Use idempotency keys for every remote mutation without automatically retrying writes.
- Preserve deterministic human, JSON, quiet, error-kind, exit-code, profile, and tenant behavior.
- Surface Xero's nested validation messages and distinguish known failures from uncertain remote outcomes.

## Non-Goals

- Batch create or batch update.
- Creating or matching contacts by name; callers provide ContactID.
- Paying invoices or bills. Xero requires an AUTHORISED invoice plus the Payments endpoint.
- Automatically merging line items with a concurrent Xero edit.
- Attachment list, download, or delete commands. Internal attachment metadata may be read for upload safety.
- Uploading attachments from stdin in v1.
- Combining invoice creation and attachment upload into one transaction-like command.
- Invoice-number lookup as an alternative to InvoiceID.
- Temporary tenant overrides; the active profile continues to select the organisation.
- A new database or local model. No ERD is needed.

## Stakeholders

- CLI users and accounting operators creating or correcting Xero transactions.
- Automation authors relying on stable JSON, idempotency, and exit codes.
- Maintainers extending the custom net/http client and Cobra command tree.
- Reviewers responsible for financial-data safety, OAuth scope behavior, and cross-platform file handling.

## Proposed Command Contract

| Workflow | Command | Required input |
| --- | --- | --- |
| Create sales invoice | xero invoices create --file invoice.json | One invoice JSON object |
| Update sales invoice | xero invoices update --invoice-id UUID --file invoice-update.json | Invoice UUID and one partial-update object |
| Upload invoice attachment | xero invoices attachments upload --invoice-id UUID --file receipt.pdf | Invoice UUID and regular local file |
| Create purchase bill | xero bills create --file bill.json | One bill JSON object |
| Update purchase bill | xero bills update --invoice-id UUID --file bill-update.json | Bill UUID and one partial-update object |
| Upload bill attachment | xero bills attachments upload --invoice-id UUID --file receipt.pdf | Bill UUID and regular local file |

All commands:

- accept no positional arguments;
- use the existing persistent --profile, --json, --quiet, and --no-browser behavior;
- resolve the tenant from the active profile token;
- expose --idempotency-key for caller-controlled retries;
- generate a cryptographically random effective idempotency key when the flag is omitted;
- never retry a mutation automatically.

### Create flags

- --file PATH|- is required. A literal dash reads JSON from stdin.
- --idempotency-key VALUE is optional and must be 1-128 bytes after trimming, with no control characters.

### Update flags

- --invoice-id UUID is required and uses the existing UUID normalization helper.
- --file PATH|- is required.
- --replace-line-items is required exactly when lineItems is present in the input.
- --idempotency-key VALUE follows the create contract.

Reject --replace-line-items when lineItems is absent. Reject a null or empty lineItems value in v1. When lineItems is present, it is the complete desired set, not a patch:

- a known LineItemID updates that line;
- no LineItemID creates a line;
- an existing LineItemID omitted from the submitted array is deleted.

The preflight read must compare current and submitted LineItemIDs and include lineItemsReplaced and removedLineItemCount in the successful result.

### Attachment upload flags

- --invoice-id UUID is required for both invoices and bills because Xero uses InvoiceID for both.
- --file PATH is required; a literal dash is rejected in v1.
- --filename NAME optionally overrides the remote filename and defaults to the source basename.
- --content-type MIME optionally overrides deterministic MIME detection.
- --overwrite opts into replacing an existing attachment with the same effective filename.
- --include-online is available only for xero invoices attachments upload, defaults false, and is valid only when creating a new attachment.
- --idempotency-key VALUE follows the create contract.

Collision behavior:

- no existing filename and no --overwrite: create with PUT;
- existing filename and no --overwrite: stop before mutation and explain --overwrite;
- existing filename and --overwrite: replace with POST;
- no existing filename and --overwrite: stop before mutation so a typo cannot silently create a file;
- --include-online with --overwrite: reject in v1 because the current OpenAPI operation exposes IncludeOnline only on create;
- ten existing attachments and a new filename: stop before mutation at Xero's documented per-document limit.

The server remains authoritative if another client changes attachments between preflight and upload.

## JSON Input Contract

### Document shape

Create and update accept one UTF-8, camelCase JSON object. They do not accept:

- Xero's outer Invoices wrapper;
- an array or more than one object;
- a UTF-8 BOM;
- trailing JSON values;
- duplicate or unknown keys;
- null values in v1;
- Xero response-only or calculated fields; or
- an input larger than a documented 1 MiB local limit.

For --file -, reject interactive terminal stdin rather than waiting indefinitely. For a path, open the file and require a regular file before loading runtime, tokens, or the network.

Implement separate create and update DTOs. Use presence-aware fields, including a pointer-to-slice or equivalent for lineItems, so omission differs from explicit false, zero, empty string, empty array, or null. Use json.Number or an equivalent lossless representation for monetary input so decoding does not introduce binary floating-point rounding before the request is encoded.

### Namespace-owned fields

The input schema does not contain type or invoiceId.

- Create injects ACCREC for invoices and ACCPAY for bills.
- Update gets InvoiceID only from --invoice-id and verifies Type through preflight. The wire DTO injects that InvoiceID and the verified Type; neither value comes from the input file.
- Explicit type or invoiceId keys are unknown-field validation errors even when their values would match.

### Supported top-level fields

| Input field | Create | Update | Notes |
| --- | --- | --- | --- |
| contactId | Required UUID | Optional UUID | Maps to Contact.ContactID; do not accept Contact name |
| date | Optional | Optional | YYYY-MM-DD |
| dueDate | Optional | Optional | YYYY-MM-DD |
| lineAmountTypes | Optional | Optional | Exclusive, Inclusive, or NoTax |
| invoiceNumber | Optional | Optional | Preserve invoice/bill semantics documented by Xero |
| reference | Optional | Optional | Maximum 255 characters |
| brandingThemeId | Optional UUID | Optional UUID | Forward only when present |
| url | Optional | Optional | Validate as an absolute HTTP(S) URL, then let Xero enforce its business rules |
| currencyCode | Optional | Optional | Validate a three-letter uppercase code |
| currencyRate | Optional number | Optional number | Must be positive |
| status | Optional; default DRAFT | Optional | Create permits DRAFT, SUBMITTED, AUTHORISED; update also permits DELETED and VOIDED; PAID is never accepted directly |
| sentToContact | Invoice only | Invoice only | Xero enforces approved-status restrictions |
| expectedPaymentDate | Invoice only | Invoice only | YYYY-MM-DD |
| plannedPaymentDate | Bill only | Bill only | YYYY-MM-DD |
| lineItems | Required, non-empty | Optional, non-empty with --replace-line-items | Complete set when present on update |

Reject invoice-only fields under bills and bill-only fields under invoices before authentication or network access.

### Supported line-item fields

- lineItemId: update only; UUID.
- description.
- quantity.
- unitAmount.
- itemCode.
- accountCode.
- accountId: UUID.
- taxType.
- taxAmount.
- lineAmount.
- discountRate and discountAmount: sales invoices only.
- tracking: at most two entries, each using either TrackingCategoryID plus TrackingOptionID or Name plus Option, without mixing selector styles in one entry.

Require at least a non-empty description for each line. Let Xero remain authoritative for organisation-specific accounts, tax codes, inventory availability, status transitions, locked periods, and paid-transaction restrictions.

### Example create documents

docs/examples/invoice-create.json:

~~~json
{
  "contactId": "eaa28f49-6028-4b6e-bb12-d8f6278073fc",
  "date": "2026-07-10",
  "dueDate": "2026-08-09",
  "lineAmountTypes": "Exclusive",
  "reference": "PO-1042",
  "status": "DRAFT",
  "currencyCode": "HKD",
  "lineItems": [
    {
      "description": "Consulting services",
      "quantity": 2,
      "unitAmount": 1500,
      "accountCode": "200",
      "taxType": "OUTPUT"
    }
  ]
}
~~~

docs/examples/bill-create.json:

~~~json
{
  "contactId": "eaa28f49-6028-4b6e-bb12-d8f6278073fc",
  "date": "2026-07-10",
  "dueDate": "2026-07-31",
  "invoiceNumber": "SUPPLIER-8831",
  "plannedPaymentDate": "2026-07-28",
  "status": "DRAFT",
  "lineItems": [
    {
      "description": "Cloud hosting",
      "quantity": 1,
      "unitAmount": 500,
      "accountCode": "404"
    }
  ]
}
~~~

docs/examples/invoice-update-lines.json must show LineItemIDs and explain that omitted existing IDs are removed when used with --replace-line-items.

## Xero API Mapping

| CLI action | Preflight | Mutation |
| --- | --- | --- |
| Create invoice/bill | None | PUT /api.xro/2.0/Invoices |
| Update invoice/bill | GET /api.xro/2.0/Invoices/{InvoiceID} | POST /api.xro/2.0/Invoices/{InvoiceID} |
| Upload new attachment | GET target invoice including attachment metadata | PUT /api.xro/2.0/Invoices/{InvoiceID}/Attachments/{FileName} |
| Replace attachment | GET target invoice including attachment metadata | POST /api.xro/2.0/Invoices/{InvoiceID}/Attachments/{FileName} |

Every request sends:

- Authorization: Bearer TOKEN;
- Xero-tenant-id from the selected profile token;
- Accept: application/json;
- Idempotency-Key on mutation requests only.

Create and update send application/json with an Invoices wrapper containing exactly one item. Attachment mutation sends the raw file stream, its detected or overridden Content-Type, and Content-Length. IncludeOnline=true appears only on new sales-invoice attachment creation.

Create injects Type. Update injects the path InvoiceID plus the Type confirmed by preflight while omitting every user field that was absent from the partial input.

Do not set SummarizeErrors=false for the single-resource v1 commands. Parse both normal error envelopes and invoice-level HasErrors, ValidationErrors, and Warnings regardless of HTTP status.

### Preflight invariants

Update and attachment upload must stop before mutation unless the GET response:

- contains exactly one invoice;
- returns the requested InvoiceID;
- returns ACCREC for the invoices namespace or ACCPAY for the bills namespace; and
- contains enough line-item or attachment metadata to apply the requested safety check.

There is no bypass flag in v1. This intentionally adds one API call to direct-ID writes.

### Semantic success invariants

HTTP 2xx is necessary but insufficient.

Create succeeds only when the response contains exactly one invoice with:

- a non-empty InvoiceID;
- the injected Type;
- no HasErrors or ValidationErrors.

Update succeeds only when the response contains exactly one invoice with:

- the requested InvoiceID;
- the preflight Type;
- no HasErrors or ValidationErrors.

Attachment upload or replacement succeeds only when the response contains exactly one attachment with:

- a non-empty AttachmentID;
- the effective filename;
- a ContentLength matching the bytes sent; and
- no validation error.

A reported validation failure is a known Xero API failure. A successful status with an empty, multiple, malformed, truncated, or identity-mismatched response is an uncertain mutation outcome because Xero may have committed the request before the response became unusable.

## Idempotency and Retry Policy

- Validate caller keys as 1-128 bytes with no control characters.
- When omitted, generate a cryptographically random 64-character hexadecimal key in the command layer before dispatch.
- Return the effective key in every success result.
- Attach the effective key and recovery command to every mutation-uncertain error.
- Never derive a key from the payload.
- Never automatically retry a write, including after token refresh has completed and the mutation request has started.
- For a 429, preserve Retry-After and instruct the user to retry the exact request with the same key.
- For an ambiguous network, 5xx, or unusable-success response, instruct the user to verify state first and reuse the same key only for the exact same request within Xero's documented six-minute idempotency window.

For create uncertainty, the recovery guidance should use the caller's invoiceNumber/reference when supplied and otherwise direct the user to list recently updated resources before retrying. Update and attachment recovery commands can use the known InvoiceID and filename.

## Attachment File Safety

Before authentication or preflight:

- open the path, then use Fstat on the open handle;
- allow a symlink only when its resolved target is a regular file;
- reject directories, devices, sockets, and FIFOs;
- reject zero-byte files;
- enforce a documented 10,000,000-byte maximum and also stream through a max-plus-one guard;
- reject a source that changes in a way that violates the declared Content-Length;
- default the remote name to the local basename;
- require --filename to be a basename, not empty, dot, or dot-dot;
- reject control characters and Xero's prohibited characters: less-than, greater-than, colon, double quote, slash, backslash, pipe, question mark, asterisk, NUL, and plus;
- URL-escape the filename as one path segment and test spaces, hash, percent, brackets, Unicode, and non-ASCII names.

MIME resolution order:

1. a valid --content-type override;
2. sniff the first 512 bytes with net/http and seek back to the start;
3. extension lookup;
4. application/octet-stream.

Do not add an arbitrary MIME allowlist. Do not expose the local source path in structured output.

## Output Contract

Preserve the existing envelope for --json and raw data for --quiet.

### Invoice and bill mutation result

~~~json
{
  "operation": "created",
  "resource": "invoice",
  "invoiceId": "220ddca8-3144-4085-9a88-2d72c5133734",
  "tenantId": "tenant-1",
  "invoiceNumber": "INV-1042",
  "type": "ACCREC",
  "status": "DRAFT",
  "updatedAt": "2026-07-10T10:00:00Z",
  "lineItemsReplaced": false,
  "removedLineItemCount": 0,
  "idempotencyKey": "generated-or-supplied-key"
}
~~~

Use operation created or updated and resource invoice or bill. Human output is one deterministic sentence containing the resource identity, tenant, and resulting status. The breadcrumb lists the exact namespace and InvoiceID.

### Attachment mutation result

~~~json
{
  "operation": "uploaded",
  "resource": "bill",
  "invoiceId": "220ddca8-3144-4085-9a88-2d72c5133734",
  "tenantId": "tenant-1",
  "type": "ACCPAY",
  "attachmentId": "e59a2c7f-1306-4078-a0f3-73537afcbba9",
  "fileName": "supplier-invoice.pdf",
  "contentType": "application/pdf",
  "bytes": 10294,
  "overwritten": false,
  "idempotencyKey": "generated-or-supplied-key"
}
~~~

Use operation uploaded or replaced. Omit includeOnline from bill results; include it as a boolean only in sales-invoice attachment results.

### Error contract additions

Keep kind, message, and exitCode stable and additive. Extend structured errors with optional fields:

- validationErrors: ordered unique Xero messages;
- mayHaveSucceeded;
- operation;
- resource;
- tenantId;
- invoiceId;
- fileName;
- idempotencyKey;
- retryAfterSeconds;
- recoveryCommand.

Add:

- MutationUncertainError with exit code 20 for outcomes that may have changed Xero;
- PermissionDeniedError with exit code 21 so HTTP 403 or insufficient scope is no longer presented as a missing login.

HTTP 401 remains AuthRequiredError. Preserve current error kinds for local validation, network failures known to occur before mutation, rate limits, request construction, and ordinary Xero validation failures.

## OAuth Scope Contract

Current 2026 granular scopes:

- create/update and their preflight: accounting.invoices;
- attachment mutation: accounting.attachments;
- attachment Type/collision preflight: accounting.invoices.read or accounting.invoices.

Legacy apps may use accounting.transactions for invoice writes until Xero's documented broad-scope retirement in September 2027. Documentation must:

- lead with granular scopes;
- mention the legacy equivalent and retirement date without making it the default;
- explain that explicit XERO_SCOPES or --scope replaces the CLI's default business scopes;
- note that default scopes already include accounting.invoices and accounting.attachments;
- tell users with old, read-only, or narrowed tokens to log in again after adding scopes.

## Technical Approach

### Architecture

Use shared factories for invoice and bill commands so Type, language, output, and validation cannot drift:

~~~go
// internal/commands/invoice_mutations.go
type invoiceCommandConfig struct {
    Namespace string
    Singular  string
    Plural    string
    Type      string
}
~~~

The command flow remains:

~~~text
local decode/file validation
  -> load runtime
  -> load and refresh token
  -> resolve profile tenant
  -> optional GET preflight
  -> one mutation request
  -> semantic response validation
  -> human/JSON/quiet output
~~~

Rename the already-broad InvoiceLister interface to InvoiceClient and add focused methods for:

- GetInvoice;
- CreateInvoice;
- UpdateInvoice; and
- UploadInvoiceAttachment.

Keep transport request DTOs distinct from the existing response-oriented Invoice model. The response model contains read-only totals, payments, allocations, and timestamps and must never be marshaled as a write payload.

Split new transport code into focused files rather than continuing to grow client.go:

- internal/xeroapi/invoices_write.go;
- internal/xeroapi/invoice_attachments.go;
- internal/xeroapi/mutation_errors.go if shared uncertainty/error decoding warrants it.

### Implementation Phase 1: Input, errors, and client foundations

- [x] Add strict single-object JSON decoding in internal/commands/json_input.go, including duplicate-key detection, UTF-8 validation, size limits, null rejection, trailing-value rejection, and stdin terminal protection.
- [x] Add create/update input types with presence-aware optional fields and json.Number monetary values.
- [x] Add namespace-aware validators for dates, UUIDs, status, line amount types, currency, invoice-only/bill-only fields, line items, and tracking.
- [x] Add idempotency-key validation and crypto/rand generation in a shared helper.
- [x] Rename InvoiceLister to InvoiceClient and update Dependencies, Runtime, fakes, and tests.
- [x] Add GetInvoice and extend normalized Invoice data with attachment metadata needed for preflight.
- [x] Expand Xero error decoding to collect nested Elements[].ValidationErrors and invoice-level ValidationErrors/Warnings.
- [x] Add mutation uncertainty and permission-denied error kinds, exit codes, and additive JSON metadata.

Success criteria:

- invalid input and local files fail before runtime/client construction;
- omitted and explicit zero/false values remain distinguishable;
- existing read commands retain their output and error contracts;
- client errors preserve all actionable Xero validation messages.

Estimated effort: 1-2 engineering days.

### Implementation Phase 2: Create and update commands

- [x] Add internal/commands/invoices_create.go and a shared factory mounted under invoices and bills.
- [x] Add internal/commands/invoices_update.go and shared Type-preflight/line-replacement checks.
- [x] Implement PUT /Invoices and POST /Invoices/{InvoiceID} in internal/xeroapi/invoices_write.go.
- [x] Wrap exactly one camelCase input object into exactly one Xero Invoices item with command-owned Type.
- [x] Set DRAFT explicitly when create status is omitted.
- [x] Validate create/update semantic results and classify unusable responses as uncertain.
- [x] Add human writers and typed compact results in internal/output/human.go and internal/xeroapi.
- [x] Add namespace-specific summaries and verification breadcrumbs.

Success criteria:

- invoice and bill create share implementation but emit the correct Type and language;
- wrong-Type IDs never reach the update endpoint;
- scalar-only updates do not include LineItems;
- line-item replacement cannot happen without the explicit flag and reports the removal count;
- every mutation sends and reports one effective idempotency key.

Estimated effort: 2-3 engineering days.

### Implementation Phase 3: Attachment upload and replacement

- [x] Add internal/commands/invoice_attachments.go with an attachments group and upload child under both namespaces.
- [x] Implement regular-file validation, remote filename validation, MIME detection, and streaming size enforcement.
- [x] Use GetInvoice preflight to verify Type, collision state, and attachment count.
- [x] Implement PUT create and POST replace in internal/xeroapi/invoice_attachments.go.
- [x] Register --include-online only for invoices and reject overwrite combinations.
- [x] Validate attachment response identity, filename, and byte count.
- [x] Add deterministic human and structured attachment results without local paths.

Success criteria:

- a duplicate filename never replaces content without --overwrite;
- --overwrite never silently creates a missing filename;
- wrong-Type IDs and an eleventh attachment fail before upload;
- the body is streamed once with correct Content-Type and Content-Length;
- invoice IncludeOnline and bill flag rejection match the documented contract.

Estimated effort: 1-2 engineering days.

### Implementation Phase 4: Tests and documentation

- [x] Update test/commands/invoices_test.go, including replacing the existing bills-have-no-actions assertion with positive command-surface tests.
- [x] Add exact wire tests to test/xeroapi/client_test.go for verbs, paths, headers, query parameters, wrappers, raw bytes, and semantic validation.
- [x] Extend test/integration/xero_invoices_integration_test.go for refresh, profile tenant, preflight, and one mutation.
- [x] Add human/JSON/quiet/error golden coverage to test/output/json_contract_test.go.
- [x] Add docs/examples/invoice-create.json, docs/examples/bill-create.json, and docs/examples/invoice-update-lines.json.
- [x] Update README.md, docs/commands/invoices.md, docs/commands/bills.md, docs/auth.md, docs/development/testing.md, and .env.example.
- [x] Add an opt-in Xero sandbox checklist under docs/test/ for real create, update, replacement, upload, overwrite, online inclusion, and cleanup.

Success criteria:

- all automated tests pass without a live Xero organisation;
- the manual checklist names every remote record it creates and how to clean it up;
- help text, examples, scopes, output, and error recovery agree.

Estimated effort: 1-2 engineering days.

## System-Wide Impact

### Interaction graph

Create:

~~~text
Cobra command
  -> strict JSON decoder and namespace validator
  -> runtime/token refresh/profile tenant
  -> Xero PUT /Invoices
  -> response semantic validator
  -> compact result and breadcrumb
~~~

Update:

~~~text
Cobra command
  -> strict JSON decoder
  -> runtime/token refresh/profile tenant
  -> Xero GET /Invoices/{ID}
  -> Type and line-item comparison
  -> Xero POST /Invoices/{ID}
  -> response semantic validator
  -> compact result and breadcrumb
~~~

Attachment:

~~~text
Cobra command
  -> open/Fstat/name/size/MIME validation
  -> runtime/token refresh/profile tenant
  -> Xero GET /Invoices/{ID}
  -> Type/count/collision decision
  -> Xero PUT create or POST replace
  -> response semantic validator
  -> compact result and breadcrumb
~~~

### Error and failure propagation

- Decoder, flag, and file-shape failures become ValidationError before auth/network.
- A file read failure after a successful open is local InternalError, not NetworkError.
- Token refresh and tenant selection retain existing typed paths.
- HTTP 401 remains AuthRequiredError.
- HTTP 403 becomes PermissionDeniedError with scope guidance.
- HTTP 429 remains RateLimitError and preserves Retry-After.
- HTTP 400 and reported validation elements become XeroApiError with ordered nested messages.
- A post-dispatch transport error, HTTP 5xx, or unusable success response becomes MutationUncertainError with mayHaveSucceeded=true, idempotency key, and recovery command.
- Output serialization failures remain local InternalError and do not imply that the remote mutation failed.

### State lifecycle risks

| Risk | Mitigation |
| --- | --- |
| Create succeeds but response is lost | Effective idempotency key, uncertain-outcome error, verify-before-retry guidance |
| Wrong namespace ID mutates a bill as an invoice or vice versa | Mandatory GET Type preflight and response Type/ID verification |
| Omitted existing line items are deleted | Presence-aware DTO, --replace-line-items gate, preflight deletion count |
| Concurrent edit occurs after preflight | No automatic merge/retry; document race and trust server validation/result |
| Same attachment filename replaces evidence | Collision preflight and required --overwrite |
| Eleventh attachment fails late | Preflight count plus server-authoritative error handling |
| Wrong tenant receives a write | Profile-selected tenant only, tenant ID in output and uncertainty metadata |
| Paid/locked invoice rejects fields | Preserve Xero validation detail; do not duplicate regional/business rules locally |
| File changes during upload | Open-handle Fstat, Content-Length, max-plus-one stream guard, byte-count verification |

### API surface parity

- invoices and bills gain create, update, and attachments upload together.
- invoices retains approve, pdf, and online-url; these are not added to bills in this scope.
- existing invoice/bill listing stays unchanged.
- future attachment list/get/delete operations can use the new attachments group without renaming upload.
- a future generic transaction abstraction must not reintroduce user-controlled Type.

### Cross-layer integration scenarios

1. Create invoice after token refresh: refresh persists, selected tenant header is used, request injects ACCREC, output contains tenant and returned ID.
2. Update bill with scalar-only input: preflight returns ACCPAY, request omits LineItems, existing lines remain, result reports no replacement.
3. Wrong namespace: xero bills update receives an ACCREC ID; preflight returns a validation error and mutation endpoint hit count stays zero.
4. Full line replacement: preflight has three IDs, input retains two and adds one; no flag fails locally, flag sends once and reports one removal.
5. Attachment collision: default upload refuses an existing name; --overwrite sends POST once; a new name sends PUT once.
6. Ambiguous create response: server accepts request then closes or returns malformed 2xx; CLI returns MutationUncertainError with the effective key and verification guidance, with no retry.

## Detailed Test Matrix

### Command tests

- commands mount under both namespaces with correct help and no positional args;
- UUID normalization and wrong/missing IDs;
- path versus stdin input, interactive stdin rejection, BOM, empty, oversized, duplicate/unknown/null/trailing JSON;
- create required fields and explicit DRAFT default;
- namespace-owned Type/InvoiceID and invoice-only/bill-only fields;
- omitted versus explicit false/zero values;
- empty update and lineItems/--replace-line-items coupling;
- idempotency key trim, byte length, controls, supplied value, and generation;
- attachment path, filename, content type, overwrite, and include-online combinations;
- every local validation failure proves no runtime/client call occurred;
- human, JSON, and quiet output.

### Client HTTP tests

- exact PUT /Invoices create and POST /Invoices/{ID} update;
- exact PUT versus POST attachment URL with escaped one-segment filename;
- Authorization, Xero-tenant-id, Accept, Content-Type, Content-Length, IncludeOnline, and Idempotency-Key;
- exact one-item Xero wrapper and absence of omitted optional fields;
- streamed body contents and server request count exactly one;
- GET preflight response includes line items and attachment metadata.

### Semantic and error tests

- nested validation errors on 400 and 2xx HasErrors responses;
- empty and multiple result arrays;
- missing or mismatched InvoiceID, Type, AttachmentID, filename, or byte count;
- malformed/truncated 2xx response becomes uncertain;
- 401, 403/insufficient scope, 429 with Retry-After, 500, 503, and context timeout;
- generated/supplied idempotency metadata on success and uncertain failure;
- no local or automatic retry.

### Attachment edge tests

- missing, unreadable, directory, FIFO/device, zero-byte, max-minus-one, max, and max-plus-one files;
- symlink to regular file;
- invalid basename and prohibited/control characters;
- spaces, hash, percent, brackets, and Unicode path escaping;
- override, sniffed, extension-derived, and fallback MIME;
- ninth/tenth versus eleventh attachment;
- collision refusal, valid replacement, overwrite-without-target rejection, and server-side race error;
- local read failure during streaming.

### Quality gates

- [x] gofmt on changed Go files.
- [x] go vet ./...
- [x] go test ./...
- [x] Existing read-only command and output tests remain unchanged or are intentionally updated.
- [x] No automated test contacts a real Xero tenant.
- [x] Manual sandbox evidence covers create, scalar update, gated line replacement, upload, collision refusal, replacement, IncludeOnline, and cleanup.
- [x] Human review focuses on mutation uncertainty, line-item deletion, tenant/type safety, scope handling, and attachment overwrite behavior.
- [x] Any AI-generated transport or validation code is reviewed against the pinned official OpenAPI paths and checked with exact-wire tests.

## Acceptance Criteria

### Functional requirements

- [x] xero invoices create and xero bills create accept one strict JSON object and return one compact typed result.
- [x] The command namespace injects ACCREC or ACCPAY; input cannot choose Type.
- [x] Create requires ContactID and a non-empty lineItems array and defaults explicitly to DRAFT.
- [x] xero invoices update and xero bills update require --invoice-id and a non-empty partial update.
- [x] Direct-ID update verifies the existing resource Type before mutation and verifies response ID/Type afterward.
- [x] Omitted lineItems preserves current lines.
- [x] Present lineItems requires --replace-line-items, cannot be empty, and reports removedLineItemCount.
- [x] Invoice and bill attachment commands stream a validated regular file to the documented Xero endpoint.
- [x] Default attachment upload refuses a duplicate filename; --overwrite replaces only an existing filename.
- [x] New sales-invoice attachment upload supports --include-online; bills do not expose the flag.
- [x] Every mutation sends and reports a valid effective Idempotency-Key.
- [x] Mutations are never automatically retried.
- [x] Xero nested validation errors are preserved and actionable.
- [x] Unusable success responses and post-dispatch failures return a distinct uncertain-outcome error with recovery metadata.
- [x] Existing profile, token refresh, tenant resolution, stdout/stderr, JSON, quiet, and breadcrumb behavior remains intact.

### Non-functional requirements

- [x] Attachment memory use is bounded and independent of file size within the 10,000,000-byte limit.
- [x] Invoice JSON input is bounded to the documented 1 MiB limit.
- [x] No token, attachment content, local file path, or full payload appears in error or structured output.
- [x] Local validation finishes before authentication or network calls.
- [x] Attachment URL path construction is safe for every allowed filename.
- [x] Rate-limit headers and insufficient-scope errors remain distinguishable from authentication failures.

## Success Metrics

- All six new command paths pass command, client, output-contract, and integration-fake tests.
- Wrong-Type preflight tests prove zero mutation requests.
- Every mutation client test proves a server hit count of exactly one.
- JSON and quiet outputs are stable enough for scripts and contain tenant, resource identity, operation, and effective idempotency key.
- A sandbox reviewer can create and clean up one draft invoice, one draft bill, one line-item update, and one attachment on each without raw HTTP.

## Dependencies and Prerequisites

- Existing browser OAuth, token refresh, profile tenant, Cobra, Viper, output, and typed error infrastructure.
- Xero granular accounting.invoices, accounting.invoices.read, and accounting.attachments scopes as appropriate.
- Current Xero Accounting API Invoices and Attachments behavior.
- Go standard-library crypto/rand, encoding/json, mime, net/http, net/url, and file-stream primitives; no Xero SDK is required.

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
| --- | --- | --- | --- |
| Xero business rules differ by organisation, region, status, or lock date | High | Medium | Validate portable syntax locally; preserve Xero validation details |
| Lost response creates duplicate manual retry | Medium | High | Idempotency key, uncertain error, verify-before-retry |
| Full line update deletes omitted lines | Medium | High | Explicit flag, preflight comparison, removal count |
| Attachment limit documentation drifts | Low | Medium | Central constant, official docs link, server error remains authoritative |
| Granular-scope migration confuses existing users | Medium | Medium | Lead docs with 2026 scopes, distinguish 403, re-login guidance |
| Preflight increases rate-limit usage | High | Low | Document two-call direct-ID mutations and honor Retry-After |
| Concurrent edit invalidates preflight snapshot | Low | High | No hidden merge/retry; surface server result and document race |
| Monolithic client becomes harder to maintain | Medium | Medium | Focused write and attachment files plus exact-wire tests |

## Alternative Approaches Considered

### Many CLI flags for invoice fields

Rejected for v1. Nested line items, tracking, optional zero values, and regional fields would produce a brittle command and ambiguous repeatable-flag syntax. A strict JSON object is scriptable and reviewable.

### Raw Xero JSON passthrough

Rejected. It would permit conflicting Type/InvoiceID, response-only fields, batches, and accidental zero-value overwrites. Dedicated request DTOs provide a stable CLI contract without marshaling the response model.

### One generic xero transactions command with --type

Rejected. The existing invoices/bills split intentionally owns ACCREC versus ACCPAY, and a user-controlled Type creates a wrong-resource mutation hazard.

### Patch-style automatic line-item merge

Rejected. It requires a read-modify-write algorithm without an Xero concurrency token and could overwrite concurrent changes. V1 makes Xero's complete-set semantics explicit.

### No attachment collision preflight

Rejected. Xero can replace a same-name attachment, which is too easy to trigger accidentally for accounting evidence.

### Automatic retry of transient write failures

Rejected. Even with idempotency support, retry timing and response ambiguity should remain visible to the caller. The CLI provides the key and recovery guidance instead.

## Resource Requirements

- One Go engineer familiar with Cobra, net/http, JSON presence semantics, and file streaming.
- One reviewer focused on accounting mutation safety and Xero behavior.
- Approximately 5-9 engineering days including automated tests, manual sandbox validation, and documentation.
- Access to a non-production Xero organisation for the opt-in manual checklist.

## Future Considerations

- Attachment list, download, and delete commands under the new attachments group.
- Batch create/update using SummarizeErrors=false only after single-resource semantics are stable.
- A dry-run payload preview that never contacts Xero.
- Invoice-number selectors with explicit zero/multiple match handling.
- Optional four-decimal unit price support through Xero's unitdp query parameter.
- Persisted idempotency/recovery records for crash resilience.
- Contact and account lookup helpers that still write ContactID/AccountCode explicitly.

## Documentation Plan

- README.md: top-level examples, capability summary, scopes, and mutation safety.
- docs/commands/invoices.md: create/update/upload usage, JSON schema, line replacement, IncludeOnline, output, recovery.
- docs/commands/bills.md: remove the list-only statement and document parallel commands and bill-specific fields.
- docs/auth.md: granular invoice/attachment scopes, explicit scope override behavior, re-login, and 401 versus 403.
- docs/development/testing.md: exact-wire and manual sandbox testing boundaries.
- .env.example: avoid a default read-only override that makes documented writes fail unexpectedly, or annotate it clearly.
- docs/examples/invoice-create.json: sales invoice create input.
- docs/examples/bill-create.json: purchase bill create input.
- docs/examples/invoice-update-lines.json: complete-set line replacement input.
- docs/test/2026-07-10-invoice-bill-write-sandbox-checklist.md: opt-in live verification and cleanup.

## Sources and References

### Internal references

- Command registration and shared invoice/bill Type config: internal/commands/invoices_list.go:37
- Bills list-only command shape to change: internal/commands/invoices_list.go:61
- Runtime, dependency injection, and output path: internal/commands/root.go:34
- Existing explicit write-command flow: internal/commands/invoices_approve.go:12
- Response-oriented invoice model and broad interface: internal/xeroapi/client.go:26 and internal/xeroapi/client.go:156
- Existing invoice list and preflight transport conventions: internal/xeroapi/client.go:321
- Existing invoice approval POST and semantic status check: internal/xeroapi/client.go:504
- Current shallow API error decoder: internal/xeroapi/client.go:567
- Current default granular invoice and attachment scopes: internal/config/config.go:324
- Stable human and JSON output: internal/output/human.go:12 and internal/output/json.go:10
- Error kinds and exit codes: internal/errors/exit_codes.go:8
- Command/client/integration/output tests: test/commands/invoices_test.go, test/xeroapi/client_test.go, test/integration/xero_invoices_integration_test.go, and test/output/json_contract_test.go
- Closest prior plan: docs/plans/2026-03-11-feat-add-invoice-approval-command-plan.md

### Related work

- PR #4, invoice approval command: https://github.com/inscoder/xero-cli/pull/4
- PR #11, invoice/bill namespace split: https://github.com/inscoder/xero-cli/pull/11
- No open or closed GitHub issues matched this feature during research.

### Official Xero references

- Invoices endpoint: https://developer.xero.com/documentation/api/accounting/invoices
- Attachments endpoint and filename/size/count rules: https://developer.xero.com/documentation/api/accounting/attachments
- Accounting types and ACCREC/ACCPAY: https://developer.xero.com/documentation/api/accounting/types/
- OAuth scopes and 2026 granular-scope migration: https://developer.xero.com/documentation/guides/oauth2/scopes/
- Idempotent requests: https://developer.xero.com/documentation/guides/idempotent-requests/idempotency/
- API limits and Retry-After: https://developer.xero.com/documentation/guides/oauth2/limits
- Accounting API response codes: https://developer.xero.com/documentation/api/accounting/responsecodes
- Integration best practices: https://developer.xero.com/documentation/guides/how-to-guides/integration-best-practices
- Official OpenAPI schema used to pin verbs, paths, parameters, and response shapes: https://github.com/XeroAPI/Xero-OpenAPI/blob/master/xero_accounting.yaml

## Research and Review Notes

- No matching docs/brainstorms requirements document existed, so this plan has no origin field.
- No docs/solutions, AGENTS.md, or CLAUDE.md guidance existed.
- Repository research and learnings research ran in parallel.
- External research was required because this feature changes remote accounting data through an evolving API.
- SpecFlow analysis added strict input presence semantics, the line-item replacement gate, wrong-Type preflight, collision-safe attachment replacement, structured uncertain outcomes, and the expanded negative test matrix.
- Historical plans were used only as prior art; current code and docs take precedence where old plans mention obsolete tenant overrides or command shapes.
