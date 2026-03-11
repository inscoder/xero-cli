---
title: feat: Add online invoice URL command
type: feat
status: completed
date: 2026-03-11
---

# feat: Add online invoice URL command

## Overview

Add a dedicated `xero invoices online-url` command that retrieves the customer-facing online invoice URL for a specific invoice by ID.

The command should use Xero's dedicated `GET /Invoices/{InvoiceID}/OnlineInvoice` endpoint, preserve the CLI's existing auth, tenant, and output conventions, and keep the current `xero invoices` list behavior intact while opening a clean path for future invoice subcommands.

## Problem Statement

The CLI already exposes an invoice `url` field inside the invoice payload returned by `xero invoices`, but Xero documents that `Url` as a source-document link shown inside Xero, not the customer-facing `OnlineInvoiceUrl`. That makes the current surface misleading for anyone trying to fetch the shareable online invoice link.

There is also no first-class command for this workflow today. Users must either inspect API docs and construct the endpoint manually or assume the existing invoice `url` field is equivalent. The new feature needs to close that gap without regressing the current `xero invoices` command, which is currently a runnable leaf command.

## Proposed Solution

Ship a focused v1 command:

```bash
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --json
```

Recommended contract decisions:

- keep `xero invoices` runnable for list behavior and add `online-url` as a child subcommand for backward compatibility
- require `--invoice-id` in v1 and validate it locally as a UUID
- call the dedicated online-invoice endpoint instead of reusing invoice `Url`
- return a small command-specific result object for structured output instead of overloading the invoice model
- treat a missing `OnlineInvoiceUrl` as a successful empty result, not a validation or transport error
- defer `--invoice-number` to a follow-up once the direct invoice-ID flow is stable; if added later, it should use exact `InvoiceNumbers` lookup plus `summaryOnly=true`

Recommended structured result shape:

```go
// internal/xeroapi/client.go
type OnlineInvoiceResult struct {
	InvoiceID        string `json:"invoiceId"`
	OnlineInvoiceURL string `json:"onlineInvoiceUrl,omitempty"`
	Available        bool   `json:"available"`
}
```

Recommended human output behavior:

- when a URL exists, print the URL plainly on stdout for easy piping and browser opening
- when no URL exists, print a short explanation such as `No online invoice URL available for invoice <id>` and exit successfully

## Technical Approach

### Architecture

Keep the existing command/runtime/client split, but add one dedicated read path for online invoice URLs:

- `internal/commands/invoices_list.go`: keep the current list behavior reachable through bare `xero invoices`
- `internal/commands/invoices_online_url.go`: add selector validation, runtime loading, auth refresh, tenant resolution, and output wiring for the new command
- `internal/commands/root.go`: continue registering `invoices` from the root, but allow that command to become both runnable and a parent namespace
- `internal/xeroapi/client.go`: add a dedicated method for `GET /Invoices/{InvoiceID}/OnlineInvoice` and a narrow response model for `OnlineInvoiceUrl`
- `internal/output/human.go` or `internal/output/online_invoice.go`: add a minimal human formatter for the new result type
- `test/commands/invoices_test.go`: cover command parsing, selector validation, backward compatibility, and structured output behavior
- `test/xeroapi/client_test.go`: cover path construction, headers, optional URL decoding, and error mapping
- `test/integration/xero_invoices_integration_test.go`: cover refresh + tenant resolution + online invoice request flow
- `docs/commands/invoices.md` and `README.md`: add usage examples and explicitly distinguish invoice `Url` from `OnlineInvoiceUrl`

Recommended client split:

```go
// internal/xeroapi/client.go
type InvoiceService interface {
	ListInvoices(context.Context, auth.TokenSet, ListInvoicesRequest) ([]Invoice, error)
	GetOnlineInvoice(context.Context, auth.TokenSet, GetOnlineInvoiceRequest) (OnlineInvoiceResult, error)
}
```

This keeps `ListInvoices` unchanged, avoids teaching callers to infer online URLs from invoice payloads, and gives tests an explicit seam for the new endpoint.

### Implementation Phases

#### Phase 1: Command shape and selector validation

Deliverables:

- reshape `invoices` so it can keep its current `RunE` while hosting a new `online-url` child command
- add `internal/commands/invoices_online_url.go` with `--invoice-id` flag parsing and UUID validation
- preserve current tenant, `--json`, `--quiet`, and `--no-browser` behavior by reusing the existing runtime path
- define the structured result contract for the command before wiring docs and tests

Success criteria:

- `xero invoices` continues to list invoices without any CLI syntax change
- `xero invoices online-url --invoice-id <uuid>` is discoverable in help output
- invalid or missing invoice IDs fail locally with `clierrors.KindValidation`

Estimated effort:

- 0.5 engineering day

#### Phase 2: Xero client integration and output contract

Deliverables:

- add a dedicated online-invoice request/response path in `internal/xeroapi/client.go`
- map `GET /Invoices/{InvoiceID}/OnlineInvoice` exactly, including `Authorization`, `Accept`, and `Xero-tenant-id`
- decode `OnlineInvoices[].OnlineInvoiceUrl` defensively and normalize it into the command result object
- add human, JSON, and quiet output handling for present and missing URLs

Success criteria:

- the client hits the documented endpoint rather than using invoice `Url`
- `--json` preserves the existing top-level envelope contract in `internal/output/json.go:13`
- `--quiet` emits raw command data only, with no extra stdout noise
- missing `OnlineInvoiceUrl` returns a stable success result with `available=false`

Estimated effort:

- 0.5-1 engineering day

#### Phase 3: Tests, docs, and follow-up hooks

Deliverables:

- expand command, client, integration, and JSON contract tests
- update `docs/commands/invoices.md` with new usage and output examples
- update `README.md` command examples and scope expectations
- add a short docs note explaining that invoice `url` is not the same as `onlineInvoiceUrl`
- leave extension points for a later `--invoice-number` follow-up without shipping the extra lookup path in v1

Success criteria:

- users can discover and use the command from repo docs alone
- tests lock in the semantic difference between invoice `Url` and `OnlineInvoiceUrl`
- `go test ./...` passes before merge

Estimated effort:

- 0.5 engineering day

## Alternative Approaches Considered

### 1. Reuse the invoice `url` field from `xero invoices`

Rejected because Xero documents `Url` as a source-document link shown inside Xero, not the customer-facing online invoice URL. Reusing it would ship the wrong behavior and make the CLI more confusing.

### 2. Add a top-level `xero invoice-url` command

Rejected because invoice operations already live under `xero invoices`. Keeping the new command in that namespace is more discoverable and fits the current CLI information architecture better.

### 3. Ship `--invoice-number` in the same change

Deferred to keep v1 small and solid. Invoice-number lookup requires a second API call, zero/multi-match handling, and additional selector rules. The invoice-ID flow is the smallest correct feature that satisfies the current request.

### 4. Resolve by `searchTerm` or raw `where`

Rejected because Xero already provides a direct online-invoice endpoint for invoice IDs, and invoice-number follow-up work should use exact `InvoiceNumbers` lookup instead of fuzzy matching.

## System-Wide Impact

### Interaction Graph

`xero invoices online-url` should follow the same runtime chain as `xero invoices`: parse flags in the command layer, call `loadRuntime(...)`, load the saved token, refresh it when needed, resolve the tenant, then call a dedicated Xero client method that performs `GET /api.xro/2.0/Invoices/{InvoiceID}/OnlineInvoice`. The result then flows through `Runtime.WriteData(...)` for JSON/quiet handling or a small human formatter for terminal output.

The only structural ripple is command registration: `internal/commands/invoices_list.go:33` currently defines `invoices` as a leaf command, so the plan must preserve bare `xero invoices` while allowing `online-url` to appear as a child.

### Error & Failure Propagation

- malformed or missing `--invoice-id` should fail locally with `clierrors.KindValidation`
- missing token, stale tenant, or refresh failure should behave exactly as current invoice commands do
- `401` and `403` from Xero should continue mapping to auth-required errors
- `429` should continue mapping to rate-limit errors
- `404` or empty online-invoice payloads need explicit handling so the command can distinguish transport failure from a valid invoice that has no online URL yet

### State Lifecycle Risks

This feature is read-only. It should not add new persisted state, migrations, or session side effects beyond the token refresh metadata that already exists today.

The main lifecycle risk is semantic drift: if the CLI continues surfacing invoice `url` and online invoice URL without clear naming and docs, users may keep treating them as interchangeable. Tests and docs need to lock that down.

### API Surface Parity

The following surfaces must stay aligned:

- root command registration in `internal/commands/root.go:103`
- existing invoice command behavior in `internal/commands/invoices_list.go:33`
- invoice client behavior and models in `internal/xeroapi/client.go:23`
- JSON envelope behavior in `internal/output/json.go:13`
- human output conventions in `internal/output/human.go:12`
- command docs in `docs/commands/invoices.md:1`
- top-level examples in `README.md:5`
- command/client/integration tests under `test/commands`, `test/xeroapi`, and `test/integration`

### Integration Test Scenarios

Cross-layer scenarios worth covering:

1. `xero invoices online-url --invoice-id <uuid> --json` refreshes the token when needed, resolves the default tenant, hits the dedicated online-invoice endpoint, and returns the stable envelope.
2. `xero invoices online-url --invoice-id <uuid>` prints only the URL on stdout when a URL exists.
3. `xero invoices online-url --invoice-id <uuid>` exits successfully with a clear message when Xero returns no `OnlineInvoiceUrl`.
4. `xero invoices online-url --invoice-id not-a-uuid` fails locally and performs no network request.
5. Bare `xero invoices --json` still behaves exactly as it does today after the command namespace reshaping.

## Acceptance Criteria

### Functional Requirements

- [x] `xero invoices online-url --invoice-id <uuid>` is available and documented.
- [x] The command calls `GET /Invoices/{InvoiceID}/OnlineInvoice` and does not reuse invoice `Url`.
- [x] Bare `xero invoices` continues to list invoices without any breaking CLI change.
- [x] `--tenant`, `--json`, `--quiet`, and `--no-browser` behave consistently with existing invoice commands.
- [x] When Xero returns an online invoice URL, the command exposes it in human and structured output.
- [x] When Xero returns no online invoice URL, the command exits successfully with a stable empty result and clear human messaging.

### Non-Functional Requirements

- [x] The top-level JSON envelope shape remains unchanged for `--json` output.
- [x] The feature introduces no new persisted config, token, or session state.
- [x] The command remains read-only and uses existing auth refresh and tenant-resolution flows unchanged.
- [x] Client code handles optional online-invoice payload fields defensively.
- [x] Help text and docs explicitly distinguish invoice `Url` from `OnlineInvoiceUrl`.

### Quality Gates

- [x] Add command tests for happy path, missing selector, invalid UUID, and backward compatibility for bare `xero invoices`.
- [x] Add client tests for endpoint path, tenant/auth headers, optional URL decoding, and error mapping.
- [x] Add or update integration coverage for refresh + online-invoice fetch.
- [x] Add JSON and quiet contract assertions for the new result shape.
- [x] Update `docs/commands/invoices.md` and `README.md` with shipped behavior.
- [x] Run `go test ./...` before merging.

## Success Metrics

- users can retrieve an online invoice URL from the CLI without manually constructing API requests
- the new command works without changing existing `xero invoices` scripts
- tests prove the CLI never confuses invoice `Url` with `OnlineInvoiceUrl`
- documentation makes the new workflow discoverable in one pass through `README.md` or `docs/commands/invoices.md`

## Dependencies & Prerequisites

- existing invoice command and runtime wiring in `internal/commands/invoices_list.go:33` and `internal/commands/root.go:103`
- current JSON envelope behavior in `internal/output/json.go:13`
- current invoice client and model definitions in `internal/xeroapi/client.go:23`
- existing command/client/integration test harnesses in `test/commands/invoices_test.go:57`, `test/xeroapi/client_test.go:17`, and `test/integration/xero_invoices_integration_test.go:21`
- Xero Accounting API invoice docs: `https://developer.xero.com/documentation/api/accounting/invoices`
- Xero online-invoice endpoint and model references: `https://xeroapi.github.io/xero-node/accounting/index.html#api-Accounting-getOnlineInvoice`

## Risk Analysis & Mitigation

| Risk | Impact | Mitigation |
| --- | --- | --- |
| Reshaping `xero invoices` into a command that also hosts subcommands breaks current behavior | High | Keep bare `xero invoices` runnable and cover it with regression tests |
| Engineers accidentally reuse invoice `Url` instead of the dedicated online-invoice endpoint | High | Add a dedicated client method and explicit tests/docs that guard the semantic distinction |
| Xero returns an empty online-invoice payload for valid invoices | Medium | Model `available=false` as a successful empty result and cover it in unit and integration tests |
| Missing scopes or tenant headers create confusing auth failures | Medium | Reuse existing auth/tenant flow and document required Xero scopes near command usage |
| Future invoice-number lookup complicates v1 unnecessarily | Medium | Keep v1 invoice-ID only, but design command/result types so invoice-number can be added later without breaking output |

## Resource Requirements

- 1 engineer familiar with Go, Cobra, and the current Xero client wiring
- approximately 1.5-2 engineering days including docs and test updates
- optional real-tenant smoke verification after local fake-based coverage passes

## Future Considerations

- add `--invoice-number` as a follow-up using exact `InvoiceNumbers` lookup plus `summaryOnly=true`
- add an `open` convenience flag or a separate `xero invoices open-online-url` workflow if browser-launch behavior becomes a common operator need
- consider a later `xero invoices list` alias if the CLI accumulates more invoice subcommands and the namespace deserves fuller symmetry

## Documentation Plan

Update these docs as part of the same change:

- `docs/commands/invoices.md`: add command usage, flags, output examples, and a note that invoice `url` is not `onlineInvoiceUrl`
- `README.md`: add a top-level example for `xero invoices online-url --invoice-id ...`
- inline help in the new command file: explain the selector and point users toward `--json` for structured output

Recommended doc examples:

```bash
# docs/commands/invoices.md
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --json

# README.md
xero invoices online-url --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734
```

## Sources & References

### Internal References

- root command registration: `internal/commands/root.go:103`
- current invoices leaf command: `internal/commands/invoices_list.go:33`
- invoice `url` field currently exposed in the invoice model: `internal/xeroapi/client.go:45`
- invoice `Url` mapped from Xero payloads today: `internal/xeroapi/client.go:153`
- current JSON output contract: `internal/output/json.go:13`
- current human invoice formatter: `internal/output/human.go:12`
- command tests for invoice behavior: `test/commands/invoices_test.go:57`
- client tests for invoice request mapping: `test/xeroapi/client_test.go:17`
- integration coverage for refresh + invoice command: `test/integration/xero_invoices_integration_test.go:21`
- related shipped plan for invoice command evolution: `docs/plans/2026-03-11-feat-xero-invoices-advanced-filtering-plan.md:10`

### External References

- Xero Accounting API invoices overview: `https://developer.xero.com/documentation/api/accounting/invoices`
- Xero online-invoice endpoint reference: `https://developer.xero.com/documentation/api/accounting/invoices#retrieving-the-online-invoice-url`
- Xero Node SDK docs for `getOnlineInvoice`: `https://xeroapi.github.io/xero-node/accounting/index.html#api-Accounting-getOnlineInvoice`
- Xero OpenAPI definitions for `getOnlineInvoice`, `InvoiceNumbers`, `summaryOnly`, and `pageSize`: `https://raw.githubusercontent.com/XeroAPI/Xero-OpenAPI/master/xero_accounting.yaml`
- Xero invoice model docs showing `Url` semantics: `https://raw.githubusercontent.com/XeroAPI/xero-node/master/src/gen/model/accounting/invoice.ts`
- Cobra command/flag guidance for backward-compatible command trees: `https://cobra.dev/docs/how-to-guides/working-with-commands/`
- General CLI UX guidance: `https://clig.dev/`

### AI-Assisted Research Notes

- local repo research identified the command, client, docs, and test touch points already used by `xero invoices`
- spec-flow analysis highlighted backward compatibility for bare `xero invoices`, the semantic mismatch between invoice `Url` and `OnlineInvoiceUrl`, and the value of deferring invoice-number lookup to a follow-up
