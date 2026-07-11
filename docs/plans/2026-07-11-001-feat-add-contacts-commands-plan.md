---
title: "feat: Add contacts commands"
type: feat
status: completed
date: 2026-07-11
---

# feat: Add contacts commands

## Overview

Add a first-class `xero contacts` command group for listing, searching, creating, and updating Xero contacts through the Accounting API.

The command surface follows the requested Xero CLI examples:

```bash
xero contacts list
xero contacts list --search "Acme"
xero contacts list --page 2 --json

xero contacts create --name "Acme Corp" --email acme@example.com --phone "+1234567890"
xero contacts create --file contact.json

xero contacts update --contact-id 00000000-0000-0000-0000-000000000001 \
  --name "Acme Corporation" --email new@acme.com
xero contacts update --file contact-update.json
```

This repository already has strict JSON decoding, profile-owned tenant selection, stable human/JSON/quiet output, idempotency keys, nested Xero validation errors, and mutation-uncertainty handling from the invoice and bill write workflows. Contacts should reuse those safety contracts instead of copying the official sample CLI's permissive JSON passthrough.

## Problem Statement

Invoices and bills require a Xero `ContactID`, but the CLI cannot currently discover or manage contacts. Users must switch to Xero's UI or write raw HTTP requests to:

- find a contact by name, email, contact number, or ID;
- create a simple customer or supplier contact;
- correct scalar contact details; or
- archive a test or obsolete contact.

Raw requests bypass profile-based tenant selection, token refresh, typed errors, output contracts, and the write safeguards added in PR #12. The requested feature should make common contact workflows convenient without introducing silent contact matching, destructive nested-array replacement, or sensitive bank/tax-data handling.

## Goals

- Add explicit `list`, `create`, and `update` children under `xero contacts`.
- Support the requested simple inline flags and strict JSON file/stdin input.
- Make flag mode and file mode mutually exclusive and deterministic.
- Provide paged contact search and exact ContactID read-back.
- Preserve omitted fields on partial update and distinguish them from explicit empty values.
- Archive contacts only through an explicit, confirmation-gated update.
- Use an effective idempotency key for every mutation without automatic retries.
- Preserve nested Xero validation errors and distinguish known failures from uncertain mutation outcomes.
- Keep default human output concise while preserving stable JSON and quiet contracts.
- Use current `accounting.contacts.read` and `accounting.contacts` OAuth scope behavior.

## Non-Goals

- Deleting contacts; Xero contact lifecycle uses `ARCHIVED`, not a delete endpoint.
- Merging contacts or setting `MergedToContactID`.
- Updating contact groups, attachments, history, or CIS settings.
- Creating or updating bank-account details, tax identifiers, default accounts, tax types, currencies, tracking categories, or payment terms in v1.
- Creating or updating nested address, phone-array, or contact-person collections in v1.
- Updating a phone number in v1. Xero does not document whether submitted `Phones` entries merge with or replace existing entries.
- Exposing `GDPRREQUEST` as a routine status transition.
- Batch create or batch update, even though the collection endpoints support multiple contacts.
- Raw Xero JSON passthrough or Xero's outer `Contacts` wrapper as CLI input.
- Contact-number path lookup, raw `POST /Contacts` update-or-create, or upsert behavior.
- Automatically matching or creating contacts from invoice/bill commands. Those commands continue to require an explicit ContactID.
- CSV or TOON output, which this Go CLI does not currently support.

## Stakeholders

- Accounting operators who need ContactIDs before creating invoices or bills.
- Automation authors who rely on stable JSON, exact identity, idempotency, and exit codes.
- Maintainers extending Cobra commands, runtime dependencies, response models, and the custom `net/http` client.
- Reviewers responsible for contact privacy, archive safety, tenant correctness, and external API behavior.

## Proposed Command Contract

| Workflow | Command | Input |
| --- | --- | --- |
| List contacts | `xero contacts list` | Optional filters and paging |
| Create with flags | `xero contacts create --name NAME [--email EMAIL] [--phone PHONE]` | One simple contact |
| Create with JSON | `xero contacts create --file PATH|-` | One strict create object |
| Update with flags | `xero contacts update --contact-id UUID <changed flags>` | One ContactID plus at least one changed field |
| Update with JSON | `xero contacts update --file PATH|-` | One strict update object containing `contactId` |

All commands:

- accept no positional arguments;
- inherit `-p, --profile`, `--json`, `--quiet`, and `--no-browser`;
- resolve the organisation exclusively from the selected profile token;
- perform local input/flag validation before config, token, tenant, or network work; and
- emit stdout/stderr using the existing runtime output rules.

`xero contacts` itself is a parent-only command. Unlike the legacy `xero invoices` parent, listing requires the explicit `list` child to match the requested command surface and official Xero CLI examples.

## `xero contacts list`

### Flags

- `--search TEXT`: maps to Xero `searchTerm`; Xero searches Name, FirstName, LastName, ContactNumber, CompanyNumber, and EmailAddress case-insensitively.
- `--contact-id UUID[,UUID...]`: repeatable/comma-separated exact ContactID filter mapped to `IDs`.
- `--page N`: positive page number, default `1` so retrieval is bounded.
- `--page-size N`: positive explicit `pageSize`; let Xero remain authoritative for its supported maximum.
- `--include-archived`: include `ARCHIVED` contacts.
- `--summary-only`: request Xero's lightweight contact projection.
- `--since YYYY-MM-DD`: maps to an RFC 1123 `If-Modified-Since` header at midnight UTC, matching invoice list behavior.
- `--where CLAUSE`: advanced Xero filter; an explicitly empty clause is rejected.
- `--order "FIELD ASC|DESC"`: optional validated order. Do not send `order` when omitted, preserving Xero's documented default.

Filters combine using Xero's intersection semantics. The docs must warn that `summaryOnly=true` cannot filter on fields omitted by the summary projection.

### Validation and transport

- Trim `--search`, `--where`, and `--order`; explicitly supplied empty values fail locally.
- Normalize ContactIDs to lowercase UUIDs and reject empty/invalid entries.
- Reuse or extract the order-field/direction validator without inheriting the invoice-specific default order.
- Send `GET /api.xro/2.0/Contacts` with `Authorization`, `Accept`, and `Xero-tenant-id`.
- Treat HTTP 304 as a successful empty contact list rather than decoding an empty body.
- Treat zero contacts as success with summary `0 contacts`.
- For non-304 2xx responses, decode the `Contacts` collection and normalized pagination metadata when present.

### Output

Human table columns:

```text
ID  NAME  EMAIL  PHONE  STATUS  CUSTOMER  SUPPLIER  UPDATED
```

The human phone column uses the first `DEFAULT` phone, falling back to the first non-empty phone. JSON/quiet output uses a repository-owned camelCase response model rather than returning raw Xero payloads.

Recommended normalized contact fields:

- `contactId`, `contactNumber`, `accountNumber`;
- `contactStatus`, `name`, `firstName`, `lastName`, `companyNumber`, `emailAddress`;
- normalized `phones` needed for read-back;
- `isSupplier`, `isCustomer`, `updatedAt`; and
- optional pagination metadata at the output-envelope level only if it can be added without breaking the existing generic envelope.

Do not expose bank-account details, tax identifiers, balances, attachments, or raw validation/status response fields in the v1 normalized contact output.

## Create Input Contract

### Flag mode

```bash
xero contacts create \
  --name "Acme Corp" \
  --email acme@example.com \
  --phone "+1234567890"
```

- `--name` is required in flag mode.
- `--email` and `--phone` are optional.
- `--phone` maps to one Xero `Phones` entry with `PhoneType: DEFAULT` and the supplied value as `PhoneNumber`.
- `--idempotency-key VALUE` is optional for an exact retry and follows the existing 1–128 byte contract.

### File mode

```bash
xero contacts create --file contact.json
printf '%s\n' '{"name":"Acme Corp","emailAddress":"acme@example.com"}' \
  | xero contacts create --file - --json
```

`contact.json`:

```json
{
  "name": "Acme Corp",
  "contactNumber": "CRM-1042",
  "accountNumber": "ACME-001",
  "firstName": "Alex",
  "lastName": "Morgan",
  "companyNumber": "12345678",
  "emailAddress": "acme@example.com",
  "phone": "+1234567890"
}
```

Supported create fields:

| Field | Required | Validation |
| --- | --- | --- |
| `name` | Yes | 1–255 characters after trimming; no `<` or `>`; no leading/trailing or repeated spaces |
| `contactNumber` | No | Maximum 50 characters |
| `accountNumber` | No | Maximum 50 characters |
| `firstName` | No | Maximum 255 characters |
| `lastName` | No | Maximum 255 characters |
| `companyNumber` | No | Maximum 50 characters |
| `emailAddress` | No | Maximum 255 characters; basic ASCII addr-spec validation |
| `phone` | No | Maximum 50 characters; maps to one DEFAULT phone |

Reject `contactId` and `contactStatus` on create. Xero creates contacts as active by default.

## Update Input Contract

### Flag mode

```bash
xero contacts update \
  --contact-id 00000000-0000-0000-0000-000000000001 \
  --name "Acme Corporation" \
  --email new@acme.com
```

Supported update flags:

- `--contact-id UUID` (required in flag mode);
- `--name`, `--email`, `--contact-number`, `--account-number`;
- `--first-name`, `--last-name`, `--company-number`;
- `--status ACTIVE|ARCHIVED`;
- `--confirm-archive`; and
- `--idempotency-key VALUE`.

Use Cobra's `Flag.Changed` state so omission differs from an explicitly supplied empty string. An empty `--email`, contact/account number, first/last name, or company number is forwarded unchanged; Xero remains authoritative about whether a particular field can be cleared. `name` can never be empty.

Do not register an update `--phone` flag in v1.

### File mode

```bash
xero contacts update --file contact-update.json
```

`contact-update.json`:

```json
{
  "contactId": "00000000-0000-0000-0000-000000000001",
  "name": "Acme Corporation",
  "emailAddress": "new@acme.com"
}
```

- `contactId` is required and must be a UUID.
- At least one mutable field beyond `contactId` is required.
- File mode accepts the same update scalars as flag mode, using `emailAddress` and `contactStatus` JSON names.
- `phone` and every nested collection are unknown-field errors on update.
- The command takes identity from `contactId`, builds `POST /Contacts/{ContactID}`, and injects the same identity into the wire object. Identity is never independently mutable.

### Input-source rules

- Exactly one data input mode is required.
- Reject `--file` combined with any scalar data flag, including `--contact-id`.
- `--idempotency-key` and `--confirm-archive` are control flags and may accompany file mode.
- Reuse `decodeJSONInput`: one camelCase UTF-8 object, maximum 1 MiB, path or piped stdin, no BOM, wrapper, array, null, duplicate/unknown keys, or trailing values.
- Use separate create and update DTOs with pointer fields for update presence semantics.

## Archive Safety

Contact status updates support only `ACTIVE` and `ARCHIVED`. Reject `GDPRREQUEST` locally.

An archive transition must satisfy all of the following:

- effective `contactStatus` is exactly `ARCHIVED`;
- `--confirm-archive` is present;
- the update contains no other mutable changes; and
- the result confirms the same ContactID and `ARCHIVED` status.

Reject `--confirm-archive` unless the effective status is `ARCHIVED`. Reactivating a contact with `ACTIVE` does not require confirmation.

This status-only rule keeps archive operations auditable and prevents a user from overlooking other edits in the same destructive command.

## Xero API Mapping

| CLI action | Method and path | Notes |
| --- | --- | --- |
| List | `GET /api.xro/2.0/Contacts` | Query/header filters; read scope permitted |
| Get exact contact internally | `GET /api.xro/2.0/Contacts/{ContactID}` | Used for exact recovery/read-back support where needed |
| Create one | `PUT /api.xro/2.0/Contacts?summarizeErrors=true` | One-item `Contacts` wrapper |
| Update one | `POST /api.xro/2.0/Contacts/{ContactID}` | One-item `Contacts` wrapper; no undocumented query parameters |

Every mutation sends:

- `Authorization: Bearer TOKEN`;
- `Xero-tenant-id` from the selected profile token;
- `Accept: application/json`;
- `Content-Type: application/json`; and
- `Idempotency-Key`.

The wire body always contains exactly one PascalCase contact:

```json
{
  "Contacts": [
    {
      "ContactID": "00000000-0000-0000-0000-000000000001",
      "Name": "Acme Corporation",
      "EmailAddress": "new@acme.com"
    }
  ]
}
```

Create uses `PUT /Contacts`, not the generic `POST /Contacts` update-or-create operation. Update uses the ID-specific endpoint and never performs upsert.

## Response and Semantic Validation

A successful mutation must contain exactly one contact and satisfy:

- non-empty `ContactID`;
- update ContactID equals the requested path ID;
- `HasValidationErrors` is false;
- `ValidationErrors` is empty; and
- an archive response reports `ContactStatus: ARCHIVED`.

A 2xx response with `HasValidationErrors` or nested validation messages is a known `XeroApiError`, not success and not mutation uncertainty.

The mutation result is compact and stable:

```json
{
  "operation": "updated",
  "resource": "contact",
  "contactId": "00000000-0000-0000-0000-000000000001",
  "tenantId": "00000000-0000-0000-0000-000000000002",
  "name": "Acme Corporation",
  "status": "ACTIVE",
  "updatedAt": "2026-07-11T10:30:00Z",
  "idempotencyKey": "caller-or-generated-key"
}
```

Human success output reports the contact name/ID, tenant, status, and effective idempotency key. Breadcrumbs use exact ID read-back:

```bash
xero contacts list --contact-id UUID --include-archived --json
```

## Idempotency, Retry, and Uncertain Outcomes

- Reuse the existing supplied/generated idempotency-key helper.
- Caller keys are trimmed, 1–128 bytes, and contain no control characters.
- Generated keys remain cryptographically random 64-character hexadecimal values.
- Never automatically retry create or update.
- Prevent `net/http` body replay by giving mutation requests a non-replayable body.
- Disable redirect following so one invocation dispatches at most one write request.

Treat these post-dispatch outcomes as `MutationUncertainError` with exit code 20:

- transport failure or context timeout;
- any 3xx response;
- HTTP 5xx;
- malformed, truncated, or trailing response JSON;
- zero or multiple returned contacts;
- missing ContactID; or
- update ContactID mismatch.

Extend `errors.Metadata` with optional `contactId`. Uncertain errors include:

- `mayHaveSucceeded: true`;
- operation and resource;
- tenant and known ContactID;
- effective idempotency key; and
- a read-only recovery command.

Update recovery is exact by ContactID. Create uncertainty normally lacks an ID, so recovery uses a safely shell-quoted search by submitted name and explicitly warns that search results require manual verification because names are not a durable identity:

```bash
xero contacts list --search 'Acme Corp' --include-archived --json
```

Xero caches idempotency outcomes for a short window and rejects reuse with a different request. Documentation must instruct callers to verify first, then reuse the same key only for an exact request retry.

## Errors and Failure Propagation

- Flag, JSON, field, mode-conflict, UUID, archive, and paging failures become `ValidationError` before auth/network.
- Token refresh and tenant selection retain existing typed paths.
- HTTP 401 remains `AuthRequiredError`.
- HTTP 403 remains `PermissionDeniedError` and should mention required contact scope/permissions where actionable.
- HTTP 429 remains `RateLimitError` and preserves `Retry-After`.
- Known Xero 4xx failures remain `XeroApiError` with ordered unique nested messages.
- Contact-level validation errors must be collected from top-level `Elements`, `Contacts`, and successful `Contacts` response elements.
- Output serialization failures remain local `InternalError`; they do not imply the remote mutation failed.

## Privacy and Permissions

Contacts can contain bank details, tax identifiers, addresses, and personal data. V1 deliberately supports and returns only the common fields needed by the requested workflows.

The plan excludes contact bank-account writes. Xero's June 29, 2026 permission change requires `BankAccountAdmin` for those writes; adding them later requires a separate permission-aware design and demo validation.

Do not log or include in errors:

- access/refresh tokens;
- tenant configuration files;
- full submitted payloads;
- local file paths;
- bank/tax fields returned unexpectedly by Xero; or
- live contact data in committed fixtures.

## OAuth Scopes

- List/get: `accounting.contacts.read` or `accounting.contacts`.
- Create/update/archive: `accounting.contacts`.

The default scope set already includes `accounting.contacts`. README, `.env.example`, and auth documentation currently show an explicit override using only `accounting.contacts.read`; update those examples so the documented write commands do not fail. Changing scopes requires re-running `xero login -p <profile>`.

## Technical Approach

### Command and runtime structure

Add:

- `internal/commands/contacts.go` for the parent and list command;
- `internal/commands/contacts_create.go`;
- `internal/commands/contacts_update.go`;
- `internal/commands/contact_input.go` and focused unit tests.

Register `newContactsCommand(deps, v)` beside invoices and bills in `NewRootCommand`.

Introduce a segregated `xeroapi.ContactClient` interface with:

```go
type ContactClient interface {
    ListContacts(context.Context, auth.TokenSet, ListContactsRequest) (ContactListResult, error)
    GetContact(context.Context, auth.TokenSet, GetContactRequest) (Contact, error)
    CreateContact(context.Context, auth.TokenSet, CreateContactRequest) (ContactMutationResult, error)
    UpdateContact(context.Context, auth.TokenSet, UpdateContactRequest) (ContactMutationResult, error)
}
```

Add `Dependencies.NewContactClient` and `Runtime.Contacts`. The concrete `xeroapi.Client` implements both `InvoiceClient` and `ContactClient`; do not append unrelated contact methods to the invoice-specific interface. Update dependency literals/fakes that build a runtime.

### Models

Keep three layers separate:

1. strict camelCase command input DTOs;
2. PascalCase write-only Contact DTOs; and
3. response payload/normalized public models.

Do not reuse the current minimal invoice-nested `contactPayload`, which only represents ContactID, Name, and ContactNumber. Do not marshal a response model into a mutation.

Suggested files:

- `internal/xeroapi/contacts.go` for list/get response models and transport;
- `internal/xeroapi/contacts_write.go` for write DTOs and mutation handling.

### Implementation Phase 1: Client and shared foundations

- [x] Add `ContactClient`, dependency/runtime wiring, and test fakes without widening `InvoiceClient`.
- [x] Add contact request, response payload, normalized model, phone, pagination, and compact mutation result types.
- [x] Extract/reuse generic UUID and optional order validation without changing invoice behavior.
- [x] Add `contactId` to additive error metadata and contact-level validation collection.
- [x] Reuse strict JSON and idempotency helpers; move helpers to neutral files only if naming makes reuse unclear.
- [x] Add exact ContactID get support for response verification/recovery infrastructure.

Success criteria:

- existing invoice/bill tests and public output remain unchanged;
- contacts have dedicated DTOs and interfaces;
- errors can carry ContactID without breaking existing JSON; and
- no contact mutation exists yet without the shared safety path.

Estimated effort: 1 day.

### Implementation Phase 2: Contact listing

- [x] Add the parent-only `xero contacts` group and `list` child.
- [x] Implement search, IDs, page, pageSize, archived, summary, since, where, and order validation.
- [x] Implement `GET /Contacts` query/header mapping and HTTP 304 handling.
- [x] Normalize contact and phone response fields without sensitive bank/tax data.
- [x] Add deterministic human table, JSON/quiet data, summary, and breadcrumbs.
- [x] Add command, exact-wire client, normalization, empty-result, and output-contract tests.

Success criteria:

- every requested list example works;
- listing always requests a bounded first page by default;
- exact ContactID read-back is scriptable;
- 304 and zero contacts are successful empty results; and
- structured output never falls back to raw Xero JSON.

Estimated effort: 1–2 days.

### Implementation Phase 3: Create and update

- [x] Implement deterministic flag/file input-mode selection and pre-auth conflict validation.
- [x] Add create/update DTOs and scalar validation with pointer-presence semantics.
- [x] Map create `phone` to one DEFAULT Xero phone and reject all phone updates.
- [x] Implement guarded ACTIVE/ARCHIVED status transitions.
- [x] Implement one-contact `PUT /Contacts?summarizeErrors=true` and `POST /Contacts/{ID}`.
- [x] Reuse non-replayable request bodies, disabled redirects, no retries, semantic result validation, and uncertainty metadata.
- [x] Add compact human/JSON/quiet mutation results and exact-ID breadcrumbs.

Success criteria:

- create requires name and returns exactly one ContactID;
- omitted update fields are absent from the request;
- flag/file mixing and phone updates fail before runtime;
- archive requires a status-only request plus confirmation; and
- each invocation dispatches no more than one mutation request.

Estimated effort: 2–3 days.

### Implementation Phase 4: Integration, documentation, and sandbox verification

- [x] Add local-fake integration tests covering token refresh, profile tenant, list, create, update, archive, and recovery metadata.
- [x] Add golden human, JSON, quiet, and error-output tests.
- [x] Add `docs/commands/contacts.md`.
- [x] Add `docs/examples/contact-create.json` and `docs/examples/contact-update.json`.
- [x] Update README.md, docs/auth.md, docs/development/testing.md, and `.env.example`.
- [x] Add a mandatory contacts demo checklist under `docs/test/`; every live Xero command in it must explicitly pass `-p demo`.
- [x] Run the checklist against `-p demo` using a unique dummy name, search and exact-ID listing, flag and file update modes, archive refusal, confirmed archive, and final archived read-back.
- [x] Record only pass/fail evidence and synthetic identifiers in the implementation handoff; never add the live tenant ID, ContactID, or response payload to the repository.

Success criteria:

- automated tests never call a real Xero tenant;
- help, docs, scopes, examples, output, and recovery commands agree;
- no live test can fall back to the default or another profile because `-p demo` is present on every Xero invocation;
- sandbox-created contacts finish in `ARCHIVED` state; and
- no live tenant/contact identifiers or payloads enter committed files.

Estimated effort: 1–2 days.

## Mandatory Demo Profile Test Run

The implementation is not complete until the automated quality gates pass and the following live workflow succeeds against the configured `demo` profile. Every live Xero invocation must contain `-p demo`; do not rely on `XERO_PROFILE` or the configured default profile.

### Guardrails

- Use only the Xero Demo Company connected to profile `demo`.
- Add `--no-browser` to the live commands so an expired/missing session fails instead of opening an interactive login unexpectedly.
- Stop before mutation if `demo` is absent, points at the wrong organisation, or lacks `accounting.contacts`.
- Use only synthetic data: a unique test name, an `example.invalid` email address, and a harmless phone number.
- Do not use real customer/supplier names, email addresses, phone numbers, bank details, or tax data.
- Keep the returned ContactID and response payload in shell variables or `/tmp` only.
- Never retry a `MutationUncertainError` until the reported read-only verification command has been run.
- Finish by archiving the created contact and confirming it remains discoverable only when archived contacts are included.

### 1. Confirm the profile and read access

```bash
xero profile list --json
xero contacts list -p demo --no-browser --page 1 --page-size 1 --json
```

The profile list must contain `demo`, and the contacts request must return successfully before any mutation. The contacts request—not a profile's default marker—is the proof that the stored demo token/tenant can be used.

### 2. Create one synthetic contact with flags

```bash
RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
CREATE_KEY="xero-cli-demo-contact-create-${RUN_ID}"

CREATE_OUTPUT="$(xero contacts create \
  -p demo \
  --no-browser \
  --name "xero-cli demo ${RUN_ID}" \
  --email "xero-cli-${RUN_ID}@example.invalid" \
  --phone "+10000000000" \
  --idempotency-key "${CREATE_KEY}" \
  --json)"

CONTACT_ID="$(printf '%s' "${CREATE_OUTPUT}" | jq -r '.data.contactId')"
test -n "${CONTACT_ID}"
test "${CONTACT_ID}" != "null"
```

Verify that the result reports `operation: created`, `resource: contact`, status `ACTIVE`, the supplied idempotency key, and a non-empty ContactID. Do not print or commit the full response as test evidence.

### 3. Verify search and exact-ID read-back

```bash
xero contacts list \
  -p demo \
  --no-browser \
  --search "xero-cli demo ${RUN_ID}" \
  --page 1 \
  --json

xero contacts list \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --json
```

Search must include the created contact, and exact-ID listing must return exactly that ContactID with status `ACTIVE`.

### 4. Exercise flag-based scalar update

```bash
UPDATE_KEY="xero-cli-demo-contact-update-flags-${RUN_ID}"

xero contacts update \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --name "xero-cli demo updated ${RUN_ID}" \
  --email "xero-cli-updated-${RUN_ID}@example.invalid" \
  --idempotency-key "${UPDATE_KEY}" \
  --json

xero contacts list \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --json
```

The result must contain the same ContactID, the updated name, status `ACTIVE`, and the supplied key. Exact-ID read-back must confirm the updated scalar values.

### 5. Exercise file-based scalar update

Build the temporary strict camelCase update object outside the repository:

```bash
UPDATE_FILE="/tmp/xero-contact-update-${RUN_ID}.json"

jq -n \
  --arg contactId "${CONTACT_ID}" \
  --arg contactNumber "CLI-${RUN_ID}" \
  '{contactId: $contactId, contactNumber: $contactNumber}' \
  > "${UPDATE_FILE}"

xero contacts update \
  -p demo \
  --no-browser \
  --file "${UPDATE_FILE}" \
  --idempotency-key "xero-cli-demo-contact-update-file-${RUN_ID}" \
  --json

xero contacts list \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --json
```

Verify the same ContactID and the new contact number through exact-ID listing. The update must not clear the name or email because those fields were omitted.

### 6. Prove archive protection fails before mutation

This command is expected to fail with `ValidationError` exit code 12 because confirmation is missing:

```bash
xero contacts update \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --status ARCHIVED \
  --idempotency-key "xero-cli-demo-contact-archive-refused-${RUN_ID}" \
  --json

xero contacts list \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --json
```

Exact-ID read-back without `--include-archived` must still show the contact as `ACTIVE`, proving that local archive validation made no remote mutation.

### 7. Archive and confirm cleanup

```bash
ARCHIVE_KEY="xero-cli-demo-contact-archive-${RUN_ID}"

xero contacts update \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --status ARCHIVED \
  --confirm-archive \
  --idempotency-key "${ARCHIVE_KEY}" \
  --json

xero contacts list \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --include-archived \
  --json
```

The mutation and read-back must both report the same ContactID with status `ARCHIVED`. This is the cleanup state because Xero does not provide a contact delete endpoint.

### 8. Remove local temporary data

```bash
rm -f "${UPDATE_FILE}"
unset CREATE_OUTPUT CONTACT_ID CREATE_KEY UPDATE_KEY ARCHIVE_KEY RUN_ID UPDATE_FILE
```

Do not place the temporary JSON file inside `docs/examples`, `testdata`, fixtures, or any tracked directory.

### Mutation uncertainty during the demo run

If create is uncertain and no ContactID is known, use the reported search recovery command with `-p demo` added explicitly:

```bash
xero contacts list \
  -p demo \
  --no-browser \
  --search "xero-cli demo ${RUN_ID}" \
  --include-archived \
  --json
```

If update/archive is uncertain, use exact-ID read-back with `-p demo --include-archived`. Retry only the exact same method/path/body with the same idempotency key after verification shows it did not take effect.

## System-Wide Impact

### Interaction graphs

List:

```text
Cobra contacts list
  -> local filter validation
  -> runtime/profile token refresh
  -> profile tenant resolution
  -> ContactClient GET /Contacts
  -> response normalization
  -> human table or JSON/quiet output
```

Create/update:

```text
Cobra contacts create/update
  -> flag/file mode selection
  -> strict local validation
  -> runtime/profile token refresh
  -> profile tenant resolution
  -> one ContactClient mutation
  -> semantic identity/validation checks
  -> compact result or recovery-aware error
```

### State lifecycle risks

| Risk | Mitigation |
| --- | --- |
| Create succeeds but response is lost | Effective idempotency key, uncertain error, search-based verification guidance |
| Update succeeds but response is lost | Exact ContactID recovery command and same-key exact retry guidance |
| Wrong contact is updated | UUID path identity, injected body identity, response identity check |
| Omitted scalar is cleared | Presence-aware DTOs and exact absent-field tests |
| Phone update deletes other phone entries | Phone updates excluded from v1 |
| Archive is accidental | ARCHIVED-only request plus `--confirm-archive` |
| Upsert creates a duplicate contact | ID-specific update endpoint; never use generic POST upsert |
| Duplicate name/number rejected | Preserve Xero validation messages; do not auto-match or retry with a new identity |
| Wrong tenant receives write | Profile-selected tenant only; tenant ID in result/uncertainty metadata |
| Sensitive fields leak | Conservative normalized model and strict write allowlist |
| Concurrent scalar edit occurs | No preflight/read-modify-write; only submitted fields are sent |

### API surface parity

- `contacts` is a new first-class noun beside invoices and bills.
- Invoice/bill input remains ContactID-only; no implicit cross-command contact creation.
- Existing global profile, output, auth, and error behavior remains shared.
- Future contact groups, attachments, history, nested collection replacement, and sensitive fields can extend the noun without changing v1 commands.

### Cross-layer integration scenarios

1. `contacts list --search Acme --page 2 --json` refreshes an expired token, uses the selected profile tenant, maps exact query parameters, and emits normalized contacts.
2. Flag create maps name/email/phone into one wrapped contact, sends one PUT with an effective key, and returns a compact result and breadcrumb.
3. File update obtains ContactID from strict camelCase JSON, omits absent fields, sends one ID-specific POST, and validates response identity.
4. An archive file without `--confirm-archive` fails before runtime; the confirmed status-only request succeeds and read-back includes archived contacts.
5. A 200 response with contact validation errors is a known Xero failure; malformed 200 and 503 responses are uncertain and never retried.

## Detailed Test Matrix

### Command tests

- parent and child command registration, help, no positional args;
- global profile/json/quiet/no-browser behavior;
- list search/where/order trimming and explicitly empty rejection;
- repeatable/comma-separated ContactID normalization;
- positive page/page-size and default page 1;
- flag mode, file path, piped stdin, and interactive stdin rejection;
- file/flag conflicts before runtime/client construction;
- missing/empty/overlong/invalid create fields;
- update ID required, at least one mutable field, and explicit empty preservation;
- create phone maps to DEFAULT and update phone is unknown/rejected;
- ACTIVE/ARCHIVED/GDPRREQUEST and archive confirmation/status-only rules;
- supplied/generated idempotency keys; and
- deterministic human, JSON, quiet, and breadcrumbs.

### Strict JSON tests

- one object only; no array or `Contacts` wrapper;
- no BOM, invalid UTF-8, duplicate/unknown/wrong-case/null/trailing data;
- 1 MiB boundary;
- `contactId` create rejection and update requirement;
- create/update supported field differences; and
- path errors do not expose local paths.

### Client exact-wire tests

- GET Contacts query encoding for every supported filter;
- If-Modified-Since RFC 1123 header and 304 handling;
- PUT collection create with `summarizeErrors=true`;
- POST ID-specific update with no undocumented query parameters;
- Authorization, tenant, Accept, Content-Type, and Idempotency-Key;
- exactly one PascalCase `Contacts` item;
- explicit empty update fields present and absent fields omitted;
- create DEFAULT phone mapping;
- response normalization and timestamps; and
- request count exactly one for all mutations.

### Semantic/error tests

- zero/multiple mutation contacts;
- missing/mismatched ContactID;
- HasValidationErrors and nested validation messages on 2xx and 4xx;
- malformed/truncated/trailing response;
- 401, 403, 404, 412, 429 with Retry-After, redirect, 500, 503, and timeout;
- mutation-uncertain metadata includes contact/tenant/key/recovery;
- shell-safe create-name recovery command; and
- no automatic transport retry.

### Quality gates

- [x] `gofmt` on changed Go files.
- [x] `go vet ./...`.
- [x] `go test -count=1 ./...`.
- [x] `go build ./cmd/xero`.
- [x] `git diff --check`.
- [x] Existing invoice/bill/auth/profile behavior remains green.
- [x] Exact-wire tests are reviewed against the pinned official OpenAPI operation paths.
- [x] Human review focuses on tenant identity, archive safety, idempotency uncertainty, sensitive output, and file/flag conflicts.
- [x] The mandatory live workflow passes with `-p demo` present on every Xero command.
- [x] Demo records are archived and live tenant/contact identifiers or payloads are not committed.

## Acceptance Criteria

### Functional requirements

- [x] `xero contacts` exposes explicit `list`, `create`, and `update` children.
- [x] List supports requested search/page examples, exact ID read-back, bounded default paging, archived contacts, summary mode, since, where, and order.
- [x] Create supports either name/email/phone flags or one strict JSON object, never both.
- [x] Create requires a valid name and maps phone to one DEFAULT phone entry.
- [x] Update supports either ContactID plus changed scalar flags or one strict JSON object containing ContactID.
- [x] Partial update preserves omitted fields and keeps explicit empty values present in the request for Xero to validate.
- [x] Update rejects phone/nested collections in v1.
- [x] ARCHIVED requires explicit confirmation and a status-only update; GDPRREQUEST is rejected.
- [x] Create uses one PUT and update uses one ID-specific POST with one-item wrappers.
- [x] Every mutation sends and reports an effective idempotency key and is never automatically retried.
- [x] Mutation responses require exactly one valid ContactID and preserve nested Xero validation messages.
- [x] Ambiguous post-dispatch outcomes return MutationUncertain exit 20 with ContactID-aware recovery metadata.
- [x] Human, JSON, quiet, summaries, breadcrumbs, stdout/stderr, profile, token-refresh, and tenant behavior follow existing contracts.

### Non-functional requirements

- [x] Local validation completes before auth/network.
- [x] JSON input is bounded to 1 MiB and rejects null/unknown/duplicate/trailing/wrapper input.
- [x] Structured output uses a stable normalized contact model rather than raw Xero JSON.
- [x] Bank, tax, balance, and unrelated sensitive contact fields are not accepted or emitted in v1.
- [x] No token, full payload, local path, or live contact identifier appears in errors or fixtures.
- [x] Read-only scope, write scope, permission failures, and rate limiting remain distinguishable.
- [x] Automated integration tests use local HTTP fakes only.
- [x] Live Xero verification uses only `-p demo`, ends with status `ARCHIVED`, and never relies on the default profile.

## Success Metrics

- All requested CLI examples are covered by command tests and documented examples.
- List/create/update exact-wire tests verify method, path, query, headers, and body.
- Every mutation uncertainty test proves the server is contacted no more than once.
- Every invalid input-mode/field/status test proves zero runtime/client calls.
- A demo reviewer can create, update through both flag and file modes, find by search and ID, prove archive refusal, and archive one dummy contact using only commands with `-p demo`.
- Invoice and bill users can discover a ContactID through `contacts list` without changing existing invoice/bill semantics.

## Dependencies and Prerequisites

- Existing Cobra/Viper command tree and profile-based runtime.
- Existing encrypted OAuth token refresh and profile tenant selection.
- Existing strict JSON, idempotency, typed error, and output infrastructure from PR #12.
- Xero `accounting.contacts.read` for list-only profiles and `accounting.contacts` for writes.
- Current Xero Accounting API Contacts and idempotency behavior.
- No new third-party Go dependency is required.

## Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
| --- | --- | --- | --- |
| Xero contact fields vary by region | High | Medium | Conservative scalar allowlist; server-authoritative business validation |
| Nested child update replaces unrelated data | Medium | High | Exclude all nested updates in v1 |
| Duplicate contact is created after lost response | Medium | High | Idempotency key, no retry, search verification |
| Archived contact is hidden during recovery | Medium | Medium | Recovery commands always include `--include-archived` |
| Archive is mistaken for delete | Medium | Medium | Explicit terminology, confirmation, status-only operation |
| Sensitive bank/tax data leaks | Low | High | Do not accept or normalize those fields in v1 |
| Explicit scope override remains read-only | High | Medium | Update examples and re-login guidance |
| Search finds multiple similar names | High | Low | Search is discovery only; mutations require ContactID |
| Raw where/summary combinations fail | Medium | Low | Document Xero restrictions and preserve API validation detail |
| API schema/prose conflicts | Medium | Medium | Prefer conservative documented intersection and exact demo tests |

## Alternative Approaches Considered

### Copy the official Xero CLI implementation

Rejected as a contract, retained as syntax inspiration. Its file mode validates only minimum fields and passes unknown JSON through, while this repository intentionally rejects unknown/response-only fields and ambiguous mutations.

### Add contact methods to `InvoiceClient`

Rejected because it would widen an invoice-specific interface and force unrelated fakes to implement contacts. A segregated `ContactClient` keeps dependencies focused.

### Rename everything to one broad AccountingClient

Deferred. It creates significant mechanical churn across invoice tests without improving this feature. Both focused interfaces can share the concrete `xeroapi.Client`.

### Use generic `POST /Contacts` for updates

Rejected because it is update-or-create and can silently upsert. ID-specific POST provides an unambiguous mutation target.

### Accept the complete Xero Contact schema

Rejected for v1. It includes regional, sensitive, permission-gated, read-only, and nested replacement fields that need separate design and sandbox evidence.

### Preflight and merge phone arrays

Rejected for v1. Child entries lack durable IDs, merge behavior is undocumented, and read-modify-write introduces a concurrency race. Excluding phone updates is safer.

### Make bare `xero contacts` list

Rejected to match the user's requested examples and official Xero CLI noun/action shape. The explicit child also leaves the parent clean for future groups.

## Future Considerations

- Contact address, phone, and contact-person replacement with explicit flags and sandbox-pinned semantics.
- Permission-aware bank account detail updates.
- Default accounting/tax/currency, tracking, and payment-term fields.
- Contact groups and attachments subcommands.
- Contact-number exact lookup.
- Explicit contact archive/reactivate convenience children if status updates prove common.
- Optional deep link to the contact in Xero.
- Shared generic list/header helpers if more Accounting API nouns are added.
- Structured pagination metadata after a broader output-envelope design.

## Documentation Plan

- `README.md`: capability summary, quick examples, scopes.
- `docs/commands/contacts.md`: full list/create/update contract, file-vs-flag modes, archive, output, recovery.
- `docs/auth.md`: contact read/write scopes, re-login, 403 permissions.
- `.env.example`: write-capable scope example.
- `docs/development/testing.md`: exact-wire and local-fake contacts coverage.
- `docs/examples/contact-create.json`: strict supported create object.
- `docs/examples/contact-update.json`: strict scalar update containing ContactID.
- `docs/test/2026-07-11-contacts-sandbox-checklist.md`: opt-in demo create/update/archive verification.

## Sources and References

### Internal references

- Root command, dependencies, runtime, and output dispatch: `internal/commands/root.go:34`, `internal/commands/root.go:49`, `internal/commands/root.go:148`, `internal/commands/root.go:255`.
- Existing invoice list validation/transport pattern: `internal/commands/invoices_list.go:81` and `internal/xeroapi/client.go:371`.
- Strict JSON input: `internal/commands/json_input.go:21`.
- Presence-aware input and idempotency helpers: `internal/commands/invoice_input.go:23`, `internal/commands/invoice_input.go:298`.
- Segregation point for a new ContactClient: `internal/xeroapi/client.go:177`.
- API error and nested validation handling: `internal/xeroapi/client.go:346`, `internal/xeroapi/client.go:629`, `internal/xeroapi/client.go:653`.
- Existing no-retry mutation transport: `internal/xeroapi/invoices_write.go:129`.
- Human success/recovery output: `internal/output/human.go:68`, `internal/output/human.go:110`.
- Stable JSON envelope: `internal/output/json.go:10`.
- Default write-capable contacts scope: `internal/config/config.go:324`.
- Invoice/bill write safety plan: `docs/plans/2026-07-10-001-feat-invoice-bill-write-workflows-plan.md`.
- Original future direct-noun direction: `docs/plans/2026-03-10-feat-xero-cli-browser-auth-invoices-plan.md:406`.
- Related merged work: PR #12, `feat: add invoice and bill write workflows`.
- No `docs/solutions/`, recent matching brainstorm, AGENTS.md, or CLAUDE.md exists in this checkout.

### Official Xero references

- Contacts endpoint: https://developer.xero.com/documentation/api/accounting/contacts
- Official Accounting OpenAPI: https://github.com/XeroAPI/Xero-OpenAPI/blob/master/xero_accounting.yaml
- Contact/address/phone types: https://developer.xero.com/documentation/api/accounting/types/
- OAuth scopes: https://developer.xero.com/documentation/guides/oauth2/scopes/
- Idempotency: https://developer.xero.com/documentation/guides/idempotent-requests/idempotency/
- Response codes: https://developer.xero.com/documentation/api/accounting/responsecodes
- Paging: https://developer.xero.com/documentation/best-practices/api-call-efficiencies/paging
- If-Modified-Since: https://developer.xero.com/documentation/best-practices/api-call-efficiencies/if-modified-since/
- Rate limits: https://developer.xero.com/documentation/best-practices/api-call-efficiencies/rate-limits
- Xero changelog, including 2026 contact bank-account permissions: https://developer.xero.com/changelog
- Official Xero CLI examples: https://github.com/XeroAPI/xero-command-line#contacts
- Official sample source: `src/commands/contacts/list.ts`, `create.ts`, `update.ts`, and `src/lib/validators.ts` in https://github.com/XeroAPI/xero-command-line

## Research and Review Notes

- The supplied feature description was detailed enough to skip interactive refinement.
- Repository and learnings research ran in parallel as required by `ce:plan`.
- External research was mandatory because this feature reads and mutates an evolving accounting API.
- `docs/solutions/` is absent, so there were no institutional solution notes to carry forward.
- SpecFlow analysis added exact input-mode rules, 304 handling, archive confirmation, ContactID recovery, sensitive-output boundaries, and the explicit exclusion of phone/nested updates.
- The plan deliberately differs from the official sample by using strict camelCase input, update presence semantics, no upsert, idempotency, no automatic retries, and mutation-uncertainty recovery.
- The official OpenAPI does not expose `summarizeErrors` on `POST /Contacts/{ContactID}`; the plan therefore adds it only to collection create.
- Xero documents scalar omission preservation but not nested collection merge/replacement, so nested updates remain out of scope until demo evidence supports a separate guarded contract.
