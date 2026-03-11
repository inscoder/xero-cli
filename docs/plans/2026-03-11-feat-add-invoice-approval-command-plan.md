---
title: feat: Add invoice approval command
type: feat
status: active
date: 2026-03-11
---

# feat: Add invoice approval command

## Overview

Add a dedicated `xero invoices approve` command that approves one Xero sales invoice by setting its status to `AUTHORISED`.

This should become the repo's first invoice write command, while preserving the existing auth refresh, tenant resolution, typed error handling, and JSON/quiet output conventions already used by `xero invoices`, `xero invoices online-url`, and `xero invoices pdf`.

## Problem Statement / Motivation

The CLI already supports listing invoices, downloading invoice PDFs, and retrieving online invoice URLs, but it does not expose a first-class workflow for approving a draft or submitted sales invoice. Today a user has to drop to custom API calls, a different tool, or the Xero UI for a common accounting action.

This feature is higher risk than the existing invoice subcommands because it changes remote accounting state. The plan needs to make the command explicit, safe in multi-tenant usage, automation-friendly, and consistent with the repo's current CLI contracts before implementation starts.

## Proposed Solution

Ship a focused v1 command:

```bash
# docs/commands/invoices.md
xero invoices approve --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices approve --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --tenant <tenant-id>
xero invoices approve --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --json
```

Recommended contract decisions:

- add `approve` as a child command under `xero invoices`; do not introduce a singular `xero invoice approve` namespace because the repo consistently uses plural noun groups under `internal/commands/invoices_list.go:33`
- keep v1 to a single explicit invoice selector with required `--invoice-id <uuid>` and local UUID validation through the existing helper pattern in `internal/commands/invoices_list.go:111`
- defer `--invoice-number` to a follow-up so v1 stays aligned with the current invoice subcommand pattern and avoids a second lookup request plus zero/multi-match handling
- reuse the existing tenant-resolution flow for consistency, but make the resolved `tenantId` visible in structured output and docs so write-side actions are not tenant-opaque
- keep the command non-interactive by default because the action is explicit and single-resource; if bulk approval is added later, that can introduce `--yes` or confirmation rules then
- add a dedicated client write method that uses Xero's invoice update contract and posts `Status: "AUTHORISED"` for one invoice
- treat a successful response whose final invoice status is `AUTHORISED` as success, even if the invoice was already approved before the request, so retries and repeated approval attempts stay operationally simple
- surface Xero validation or business-rule failures directly instead of inventing local status-transition rules that could drift from upstream behavior

Recommended request/response contract:

```go
// internal/xeroapi/client.go
type ApproveInvoiceRequest struct {
	TenantID  string
	InvoiceID string
}

type InvoiceApprovalResult struct {
	InvoiceID      string `json:"invoiceId"`
	TenantID       string `json:"tenantId"`
	InvoiceNumber  string `json:"invoiceNumber,omitempty"`
	Type           string `json:"type,omitempty"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updatedAt,omitempty"`
	StatusObserved bool   `json:"statusObserved"`
}
```

Recommended human output behavior:

- print a one-line success message such as `Approved invoice INV-0001 (220ddca8-3144-4085-9a88-2d72c5133734) for tenant tenant-1`
- when Xero returns the invoice ID but no invoice number, fall back to the UUID in the human message
- keep diagnostics and auth prompts on stderr, with only the success line or JSON payload on stdout

Recommended Xero wire contract for v1:

```json
{
  "Invoices": [
    {
      "InvoiceID": "220ddca8-3144-4085-9a88-2d72c5133734",
      "Status": "AUTHORISED"
    }
  ]
}
```

Use `POST /api.xro/2.0/Invoices` for the mutation because it matches the existing reference implementation, keeps the body format aligned with Xero's collection-style invoice update contract, and leaves room for future command-local write methods without introducing a second path shape in the client.

## Technical Considerations

- **Architecture impacts**
  - add `internal/commands/invoices_approve.go` for flag parsing, selector validation, runtime loading, auth refresh, tenant resolution, and output wiring
  - register `newInvoicesApproveCommand(deps, v)` beside the existing subcommands in `internal/commands/invoices_list.go:106`
  - extend `internal/xeroapi/client.go` with `ApproveInvoice(...)`, `ApproveInvoiceRequest`, and `InvoiceApprovalResult`
  - extend the `xeroapi.InvoiceLister` interface so command tests can fake the new write path the same way they fake list, PDF, and online-URL paths today
  - add a small human formatter in `internal/output/human.go` or a command-local writer for approval success text
  - update `docs/commands/invoices.md` and `README.md` so the shipped command is discoverable in the same places as other invoice subcommands
- **Performance implications**
  - the happy path should remain a single mutation request plus normal token refresh behavior when required
  - avoid a mandatory preflight `GET` to fetch current invoice state in v1; it adds latency, increases API usage, and is not required to perform the documented approval operation
  - keep JSON output narrow and command-specific instead of returning the full invoice list model when only mutation confirmation is needed
- **Security considerations**
  - this is the first invoice write path, so tenant safety and scope documentation matter more than for read-only commands
  - support the existing `--tenant` override and clearly document that users should pass it explicitly when approving outside the saved default org
  - document required scopes for both legacy and granular Xero apps: `accounting.transactions` or `accounting.invoices`
  - keep the command single-invoice only in v1 so there is no bulk-approval blast radius and no need for confirmation prompts

## System-Wide Impact

- **Interaction graph**: `xero invoices approve` should parse `--invoice-id`, call `loadRuntime(...)`, load the saved token, refresh it through `EnsureToken(...)`, resolve the tenant using the same path as the existing invoice commands, then call a dedicated `ApproveInvoice(...)` client method. The decoded result should flow through `Runtime.WriteData(...)` so `--json` and `--quiet` stay on the shared envelope path.
- **Error propagation**: invalid or missing `--invoice-id` should fail locally with `clierrors.KindValidation`. Missing tokens, refresh failures, revoked tenants, `401`/`403`, and `429` should keep the same mappings already used in `internal/xeroapi/client.go:486`. Xero validation failures for non-approvable invoices should stay actionable and should not be collapsed into a generic success or retry message.
- **State lifecycle risks**: unlike the existing invoice subcommands, this command changes remote invoice state. The biggest risk is ambiguous success when the network fails after request dispatch. The plan should keep the mutation single-invoice and add breadcrumbs that point to `xero invoices --invoice-id <id> --tenant <tenant> --json` so the user can verify final state immediately.
- **API surface parity**: the new command must stay aligned with root persistent flags in `internal/commands/root.go:87`, JSON output rules in `internal/output/json.go:13`, human output conventions in `internal/output/human.go:12`, invoice docs in `docs/commands/invoices.md:1`, top-level examples in `README.md:5`, and the existing command/client/integration/output test harnesses.
- **Integration test scenarios**:
  1. `xero invoices approve --invoice-id <uuid> --json` refreshes the token when needed, resolves the default tenant, posts the documented Xero payload, and emits the stable JSON envelope.
  2. `xero invoices approve --invoice-id <uuid>` prints a single human success line and no unexpected stderr output on success.
  3. `xero invoices approve --invoice-id not-a-uuid` fails locally and never calls the Xero client.
  4. `xero invoices approve --invoice-id <uuid> --tenant tenant-2 --json` uses the explicit tenant override instead of the saved default.
  5. a non-approvable invoice response from Xero returns a typed CLI failure and does not produce a success envelope.

### Detailed Design Notes

#### Command contract details

- `xero invoices approve` should mirror the existing invoice child-command shape: `Use: "approve"`, `Short` text that clearly says it authorizes one invoice, and `Args: cobra.NoArgs`
- `--invoice-id` should be required at the Cobra layer and then revalidated through the existing UUID helper so help output, shell completion, and local validation all stay predictable
- the command should not accept positional invoice selectors in v1; that keeps the write path explicit and avoids ambiguity with invoice numbers
- `--quiet` should emit only the raw `InvoiceApprovalResult` payload, matching the repo's existing output-mode expectation for automation-oriented callers
- `--json` should continue using the standard top-level envelope so downstream tooling does not need a special-case parser for the first invoice mutation

#### Client and response mapping details

- add a dedicated `ApproveInvoice(context.Context, auth.TokenSet, ApproveInvoiceRequest) (InvoiceApprovalResult, error)` method instead of overloading `ListInvoices`; this keeps read and write semantics distinct in both tests and future command composition
- keep the wire payload minimal by sending only `InvoiceID` and `Status`; do not mirror the full invoice object or include unrelated fields that could trigger accidental updates
- decode the first invoice from the Xero `Invoices` collection response and fail with a typed upstream error if the response is empty or malformed
- normalize the result into the command-specific struct even when optional fields such as invoice number or updated timestamp are absent
- set `StatusObserved` to `true` only when the response includes an invoice whose final status can be confirmed as `AUTHORISED`

#### Error and retry behavior

- preserve existing auth, scope, rate-limit, and tenant-resolution error mapping so the new command feels like an extension of the current CLI rather than a special-case subsystem
- if the HTTP request fails before a response is received, report the error as a normal upstream failure and include a breadcrumb that points to a follow-up `xero invoices --invoice-id ... --tenant ... --json` verification command
- if Xero returns validation failures such as wrong invoice type, invalid transition, missing permissions, or organization capability restrictions, surface Xero's message detail directly when available
- do not add local retry loops for the mutation in v1; automatic retries on write paths are higher risk and can be revisited once the repo has a broader mutation strategy

#### Test design notes

- command tests should assert both required-flag behavior and the exact request object passed into the mocked invoice service
- client tests should assert method, path, headers, and JSON body, plus success decoding when optional invoice fields are missing
- integration tests should continue to use the fake HTTP server pattern so token refresh, tenant resolution, and invoice mutation behavior are exercised together
- output-contract tests should pin both envelope mode and quiet mode so later write commands can follow the same mutation-result pattern

### Implementation Phases

#### Phase 1: Command shape and validation

Deliverables:

- register `newInvoicesApproveCommand(deps, v)` under the existing `xero invoices` namespace
- add `internal/commands/invoices_approve.go` with `--invoice-id` parsing, UUID validation, tenant resolution, and runtime wiring
- define the command-local result handling and success summary text before the client call is wired
- extend the relevant client-facing interface so command tests can inject a fake approval implementation

Success criteria:

- `xero invoices approve --help` is discoverable and consistent with other invoice child commands
- missing or malformed `--invoice-id` fails locally with `clierrors.KindValidation`
- the command follows the existing runtime chain without introducing alternate auth or output plumbing

Estimated effort:

- 0.5 engineering day

#### Phase 2: Xero mutation path and output contract

Deliverables:

- add `ApproveInvoiceRequest`, `InvoiceApprovalResult`, and `ApproveInvoice(...)` in `internal/xeroapi/client.go`
- implement `POST /api.xro/2.0/Invoices` with the documented `AUTHORISED` payload and existing auth headers
- decode the Xero response into the dedicated approval result type and wire it through shared JSON and quiet output flows
- add a human-readable success formatter that includes invoice identity, tenant identity, and resulting status

Success criteria:

- outbound requests match the planned Xero mutation contract exactly
- a response whose final status is `AUTHORISED` is treated as success, including already-approved invoices
- `--json` and `--quiet` remain compatible with the repo's existing output contracts

Estimated effort:

- 0.5-1 engineering day

#### Phase 3: Tests, docs, and verification breadcrumbs

Deliverables:

- add or expand command, client, integration, and JSON-contract tests for the new write path
- update `docs/commands/invoices.md` and `README.md` with usage, scopes, tenant guidance, and verification examples
- document the recommended post-mutation verification command for uncertain network outcomes
- run `go test ./...` and fix any regressions introduced by the new interface surface

Success criteria:

- repo docs are sufficient for a user to discover and safely use the command
- tests cover both happy-path approval and representative validation/business-rule failures
- the feature establishes a reusable precedent for future invoice write commands

Estimated effort:

- 0.5 engineering day

## Alternative Approaches Considered

### 1. Add approval as a flag on `xero invoices`

Rejected because mixing read and write behavior into the listing command would make the CLI more surprising, complicate help output, and weaken safeguards around explicit invoice selection.

### 2. Preflight the invoice with a `GET` before approving

Rejected for v1 because it adds latency and API usage without guaranteeing better safety. Xero remains the source of truth for whether a transition is allowed, and the write call already returns the resulting invoice state.

### 3. Support `--invoice-number` alongside `--invoice-id`

Deferred because it would require an additional lookup request, ambiguity handling for zero or multiple matches, and more complex UX for the repo's first invoice mutation. The simpler UUID-only contract is safer for v1.

### 4. Treat already-approved invoices as a no-op warning instead of success

Rejected because retries and repeated approval attempts are operationally simpler when any response that confirms final `AUTHORISED` state is considered successful.

## Acceptance Criteria

- [ ] `xero invoices approve --invoice-id <uuid>` is available and documented in `docs/commands/invoices.md` and `README.md`
- [ ] the command lives under `xero invoices` rather than introducing a new singular namespace
- [ ] local validation rejects missing or malformed `--invoice-id` before any network call
- [ ] the command follows the repo's established runtime chain: `loadRuntime(...)` -> `LoadToken()` -> `EnsureToken()` -> tenant resolution -> Xero client -> `WriteData(...)`
- [ ] the Xero client sends `POST /api.xro/2.0/Invoices` with `Authorization`, `Accept: application/json`, and `Xero-tenant-id` headers, plus a body that sets `Status` to `AUTHORISED` for the selected invoice
- [ ] the implementation returns a dedicated structured result type for approval instead of overloading the list-invoices payload
- [ ] `--json` preserves the existing top-level envelope contract and `--quiet` emits only the raw approval result
- [ ] success output includes enough information to confirm what changed, at minimum `invoiceId`, `tenantId`, and resulting `status`
- [ ] a response whose final invoice status is `AUTHORISED` is treated as success for v1, even if the invoice may already have been approved before the call
- [ ] non-sales invoices, invalid transitions, missing scopes, and other Xero validation/business-rule failures are surfaced as actionable CLI errors
- [ ] docs call out required scopes and recommend explicit `--tenant` usage for non-default organizations
- [ ] tests cover command, client, integration, and output-contract behavior, and `go test ./...` passes before merge

## Success Metrics

- users can approve a sales invoice from the CLI without building a custom API request
- the feature adds write-side functionality without breaking existing `xero invoices`, `xero invoices online-url`, or `xero invoices pdf` behavior
- support and debugging remain straightforward because success output always identifies the invoice and tenant involved
- tests establish a stable precedent for future write-side Xero commands in this repo

## Dependencies & Risks

- **Key dependencies**
  - invoice command namespace and UUID validation helpers in `internal/commands/invoices_list.go:33`
  - shared runtime/auth/output flow in `internal/commands/root.go:155`
  - Xero client request/error mapping in `internal/xeroapi/client.go:303`
  - JSON contract writer in `internal/output/json.go:13`
  - current human output writers in `internal/output/human.go:12`
  - command tests in `test/commands/invoices_test.go:89`
  - integration pattern in `test/integration/xero_invoices_integration_test.go:22`
  - JSON contract tests in `test/output/json_contract_test.go:11`
- **Primary risks**
  - this is the repo's first invoice mutation, so wrong-tenant execution is materially more harmful than for list/read commands
  - Xero scope mismatches may surface as auth-like failures, which can confuse users unless docs and errors mention invoice-write scopes explicitly
  - network failure after request dispatch can leave the final remote state uncertain, so success breadcrumbs and follow-up verification matter
  - Xero's invoice update responses may include validation detail that should be preserved rather than reduced to a generic upstream message

## Implementation Suggestions

Recommended file touch points:

- `internal/commands/invoices_list.go`: register `newInvoicesApproveCommand(deps, v)` next to `newInvoicesPDFCommand(deps, v)` and `newInvoicesOnlineURLCommand(deps, v)`
- `internal/commands/invoices_approve.go`: add `Use: "approve"`, `Args: cobra.NoArgs`, `--invoice-id`, selector validation, runtime orchestration, breadcrumbs, and human/JSON output wiring
- `internal/xeroapi/client.go`: add request/result structs, extend the interface, build the `POST /Invoices` request, and decode the updated invoice response into the approval result
- `internal/output/human.go`: add a dedicated approval success writer if command-local string formatting becomes noisy
- `test/commands/invoices_test.go`: add happy path, missing flag, invalid UUID, tenant override, and typed-upstream-error tests
- `test/xeroapi/client_test.go`: add request-path, header, body, response-decoding, and error-mapping coverage for the mutation path
- `test/integration/xero_invoices_integration_test.go`: add refresh + tenant + invoice-approve mutation coverage using the fake HTTP server pattern already used for list, online URL, and PDF
- `test/output/json_contract_test.go`: add envelope and quiet assertions for `InvoiceApprovalResult`
- `docs/commands/invoices.md`, `README.md`, and optionally `docs/auth.md`: document the new command, scope expectations, and verification workflow

Suggested v1 JSON example:

```json
{
  "ok": true,
  "data": {
    "invoiceId": "220ddca8-3144-4085-9a88-2d72c5133734",
    "tenantId": "tenant-1",
    "invoiceNumber": "INV-1000",
    "type": "ACCREC",
    "status": "AUTHORISED",
    "updatedAt": "2026-03-11T12:30:00Z",
    "statusObserved": true
  },
  "summary": "invoice approved",
  "breadcrumbs": [
    {
      "action": "show",
      "cmd": "xero invoices --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --tenant tenant-1 --json"
    }
  ]
}
```

## Scope Boundaries

In scope for v1:

- approve one invoice by explicit invoice ID
- support the existing persistent flags `--tenant`, `--json`, `--quiet`, and `--no-browser`
- keep the repo's current auth refresh, tenant resolution, error mapping, and output-mode conventions intact
- document invoice-write scopes and verify behavior with command, client, integration, and JSON-contract tests

Out of scope for v1:

- `xero invoice approve` as a separate singular namespace or alias
- approve by invoice number, search term, filter, or batch input
- generic status updates beyond `AUTHORISED`
- approval confirmation prompts, `--yes`, or interactive write safeguards for multi-resource operations
- organization-capability preflight checks such as `CreateApprovedInvoice`
- local persistence of idempotency keys or automatic retry orchestration

## Open Questions To Flag During Implementation

- should the implementation preserve richer Xero validation detail in the typed CLI error path by extending `decodeAPIError(...)`, or is the current top-level message mapping sufficient for v1?
- does the chosen Xero response payload reliably include enough fields to populate `invoiceNumber`, `type`, and `updatedAt`, or should `InvoiceApprovalResult` degrade gracefully to `invoiceId`, `tenantId`, and `status` only?
- if Xero starts requiring granular scopes for all new apps, should the repo's docs be updated globally from `accounting.transactions` to `accounting.invoices` at the same time as this feature?

## Sources & References

### Internal References

- root command registration and persistent flags: `internal/commands/root.go:76`
- invoice namespace, child-command registration, and UUID helper: `internal/commands/invoices_list.go:33`
- existing subcommand pattern for invoice child commands: `internal/commands/invoices_online_url.go:13`
- runtime JSON vs human output split: `internal/commands/root.go:195`
- JSON envelope contract: `internal/output/json.go:13`
- human output helpers: `internal/output/human.go:12`
- invoice command docs: `docs/commands/invoices.md:1`
- top-level CLI examples and scope docs: `README.md:3`
- testing guidance: `docs/development/testing.md:1`
- related prior art: `docs/plans/2026-03-11-feat-add-online-invoice-url-command-plan.md:10`
- related prior art: `docs/plans/2026-03-11-feat-add-invoice-pdf-command-plan.md:10`

### External References

- Xero invoices API documentation: `https://developer.xero.com/documentation/api/accounting/invoices`
- Xero invoice status UX guidance: `https://developer.xero.com/documentation/best-practices/user-experience/invoice-status/`
- Xero requests and responses guide: `https://developer.xero.com/documentation/api/accounting/requests-and-responses`
- Xero response codes and error behavior: `https://developer.xero.com/documentation/api/accounting/responsecodes`
- Xero OAuth scopes guidance: `https://developer.xero.com/documentation/guides/oauth2/scopes/`
- Xero granular scopes FAQ: `https://developer.xero.com/faq/granular-scopes`
- Xero organization actions capability reference: `https://developer.xero.com/documentation/api/accounting/organisation`
- Xero OAuth limits guidance: `https://developer.xero.com/documentation/guides/oauth2/limits/`
- Command Line Interface Guidelines: `https://clig.dev/`

### Related Work

- no matching brainstorm document found under `docs/brainstorms/`
- no `CLAUDE.md` file was present in the repository during local research
- no related issue or PR reference was discovered during local research

### AI-Assisted Research Notes

- local repo research showed the codebase already has a strong pattern for invoice subcommands, but no invoice write path yet
- external research confirmed that Xero approval maps to setting invoice status to `AUTHORISED` and that 2026 scope expectations may differ between legacy and granular-scope apps
- SpecFlow analysis highlighted the two most important write-path decisions for v1: keep the command single-invoice and make tenant behavior explicit in both docs and output
