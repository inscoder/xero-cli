---
title: feat: Add invoice PDF command
type: feat
status: completed
date: 2026-03-11
---

# feat: Add invoice PDF command

## Overview

Add a dedicated `xero invoices pdf` command that retrieves a single invoice PDF from Xero by invoice ID and writes it to an explicit destination.

This should extend the existing `xero invoices` namespace in the same way `xero invoices online-url` does, while preserving the CLI's current auth refresh, tenant resolution, error mapping, and output-mode conventions. Because this is the first binary download path in the repo, the plan should keep the v1 contract narrow and explicit.

## Problem Statement / Motivation

The CLI already supports invoice listing and online invoice URL lookup, but it does not provide a first-class way to retrieve the actual invoice PDF. Today, a user has to drop to `curl`, browser downloads, or custom API code for a common accounting workflow.

This feature also introduces a new kind of response that the repo does not handle yet: binary PDF bytes instead of JSON. That creates design pressure around output modes, stdout safety, file writes, and test coverage. The plan needs to solve those decisions up front so the implementation does not accidentally corrupt terminal output or invent a one-off contract that fights the rest of the CLI.

## Proposed Solution

Ship a focused v1 command:

```bash
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output -
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf --json
```

Recommended contract decisions:

- add `pdf` as a child command under `xero invoices` so the namespace stays consistent with `online-url`
- require `--invoice-id` in v1 and validate it locally as a UUID before any network call
- require `-o, --output <path|->` in v1 so remote download and file creation stay explicit
- stream bytes instead of buffering the entire PDF in memory
- preserve `--json` and `--quiet` only for file-output mode by returning metadata about the saved artifact, not the PDF bytes themselves
- reject `--json` or `--quiet` when `--output -` is used, because stdout is already reserved for binary content in that mode
- reject raw PDF output to an interactive terminal unless the user explicitly pipes stdout through `--output -`
- defer `--invoice-number`, automatic filename generation, browser-opening, batch downloads, and quote/credit-note parity to follow-up work

Recommended metadata contract for file-output mode:

```go
// internal/xeroapi/client.go
type InvoicePDFResult struct {
	InvoiceID   string `json:"invoiceId"`
	ContentType string `json:"contentType"`
	Bytes       int64  `json:"bytes"`
	Output      string `json:"output"`
	SavedTo     string `json:"savedTo,omitempty"`
	Streamed    bool   `json:"streamed"`
}
```

Recommended human output behavior:

- when writing to a file, print a short success message such as `Saved invoice PDF to invoice.pdf (48213 bytes)`
- when streaming to stdout, emit only raw PDF bytes and keep diagnostics on stderr
- when stdout is an interactive terminal and the user requests `--output -`, fail locally with a clear message telling them to use a file path or pipe the output

## Technical Considerations

- **Architecture impacts**
  - add `internal/commands/invoices_pdf.go` for selector validation, destination validation, runtime loading, auth refresh, tenant resolution, and command-specific output branching
  - extend `internal/xeroapi/client.go` with a dedicated PDF retrieval method and narrow request/result types instead of overloading invoice list models
  - keep `internal/commands/root.go:76` and `internal/commands/invoices_list.go:33` as the command-registration path, adding `pdf` alongside `online-url`
  - add a small human formatter or command-local success writer for saved-file output; do not route raw PDF streaming through `output.WriteJSON`
- **Performance implications**
  - stream `resp.Body` directly to the destination writer to avoid holding full PDFs in memory
  - verify `Content-Type` and byte count while streaming so tests can distinguish valid PDF responses from accidental JSON or HTML error bodies
- **Security considerations**
  - invoice PDFs can contain sensitive customer and billing data, so avoid base64-in-JSON output and avoid printing bytes to an interactive terminal
  - keep diagnostics off stdout and prefer atomic file writes with temp file + rename for normal file mode
  - preserve existing auth and tenant flows unchanged so the command does not introduce new credential handling paths

## System-Wide Impact

- **Interaction graph**: `xero invoices pdf` should parse flags in `internal/commands/invoices_pdf.go`, call `loadRuntime(...)`, load the saved token, refresh it when needed, resolve the tenant, then call a dedicated Xero client PDF method. From there the flow branches: file mode writes bytes to a temp file and then emits metadata through the normal writer path, while stdout mode writes binary bytes directly to `rt.IO.Out` and bypasses JSON-envelope helpers.
- **Error propagation**: invalid UUIDs, empty `--output`, or incompatible flag combinations should fail locally with `clierrors.KindValidation`. Auth refresh, tenant resolution, `401`/`403`, `429`, and generic Xero API failures should continue using the existing mappings in `internal/xeroapi/client.go:394`. File-system failures need a stable CLI error message path as well.
- **State lifecycle risks**: the feature is read-only from Xero's perspective, but it introduces local file creation. Partial writes or interrupted downloads must not leave a half-written target file in place; use temp-file cleanup and rename semantics for file mode. Refresh metadata should remain the only persisted state side effect.
- **API surface parity**: the new command must stay aligned with root persistent flags in `internal/commands/root.go:87`, existing invoice subcommand docs in `docs/commands/invoices.md:35`, top-level examples in `README.md:7`, and the command/client/integration/output test harnesses already used by `online-url`.
- **Integration test scenarios**:
  1. `xero invoices pdf --invoice-id <uuid> --output out.pdf --json` refreshes the token when needed, resolves the default tenant, writes a PDF file, and emits stable metadata JSON.
  2. `xero invoices pdf --invoice-id <uuid> --output out.pdf` writes the file atomically and prints a short human success message.
  3. `xero invoices pdf --invoice-id <uuid> --output -` writes raw bytes to stdout in non-interactive mode and emits no extra stdout text.
  4. `xero invoices pdf --invoice-id <uuid> --output - --json` fails locally before any network request.
  5. `xero invoices pdf --invoice-id not-a-uuid --output out.pdf` fails locally and never calls the Xero client.

## Acceptance Criteria

- [x] `xero invoices pdf --invoice-id <uuid> --output <path|->` is available and documented in `docs/commands/invoices.md` and `README.md`
- [x] the command follows the existing runtime flow: load token, refresh if needed, resolve tenant, call Xero, and persist refresh/session updates exactly as current invoice commands do
- [x] the Xero client implements a dedicated invoice-PDF request using Xero's PDF retrieval contract, including `Authorization`, `Xero-tenant-id`, and `Accept: application/pdf`
- [x] local validation rejects missing or invalid `--invoice-id`, missing `--output`, and incompatible combinations such as `--output - --json` or `--output - --quiet`
- [x] file-output mode streams bytes to disk using atomic write semantics and returns metadata that fits the existing JSON-envelope and quiet-output conventions
- [x] stdout mode streams raw PDF bytes only and never mixes human text or JSON into the same stdout stream
- [x] the command refuses to dump binary PDF bytes to an interactive terminal and provides a clear recovery message
- [x] non-2xx Xero responses preserve the repo's existing error-kind mapping behavior
- [x] successful responses validate or at least inspect `Content-Type` so the command does not silently save a JSON error body as a `.pdf`
- [x] tests cover command, client, integration, and output-contract behavior, and `go test ./...` passes before merge

## Success Metrics

- users can retrieve an invoice PDF from the CLI without manual `curl` commands or browser navigation
- file-output mode works in one command with predictable saved-path feedback
- piping mode works for advanced users without corrupting interactive terminals
- tests lock in the binary-output rules so future invoice subcommands do not reintroduce unsafe stdout behavior

## Dependencies & Risks

- **Key dependencies**
  - existing invoice command registration in `internal/commands/invoices_list.go:33`
  - runtime output switching in `internal/commands/root.go:195`
  - JSON envelope behavior in `internal/output/json.go:13`
  - online-invoice prior art in `internal/commands/invoices_online_url.go:13`
  - command/client/integration test harnesses in `test/commands/invoices_test.go:203`, `test/xeroapi/client_test.go:133`, and `test/integration/xero_invoices_integration_test.go:200`
- **Primary risks**
  - this is the repo's first binary download path, so it is easy to accidentally force binary data through a JSON-only abstraction
  - Xero's PDF docs have a small contract ambiguity: public SDK/OpenAPI references show `/Invoices/{InvoiceID}/pdf`, while Xero's broader invoice docs also describe PDF retrieval via `Accept: application/pdf` on the invoice resource; implementation should verify the exact working wire contract and lock it down with tests
  - file writes introduce partial-write and overwrite behavior that the current invoice commands do not need to handle
  - future pressure to add `--invoice-number` or quote/credit-note parity can quickly expand scope if v1 boundaries are not kept narrow

## Implementation Suggestions

Recommended file touch points:

- `internal/commands/invoices_list.go`: register `newInvoicesPDFCommand(deps, v)` next to `newInvoicesOnlineURLCommand(deps, v)`
- `internal/commands/invoices_pdf.go`: add flag parsing, local validation, runtime orchestration, TTY guardrails, and human/JSON branching
- `internal/xeroapi/client.go`: add `GetInvoicePDF(...)` plus request/result types and binary-response handling
- `internal/output/human.go` or `internal/output/invoice_pdf.go`: add human formatter for file-save success output if that keeps command code cleaner
- `test/commands/invoices_test.go`: add happy path, validation, TTY safety, and flag-combination tests
- `test/xeroapi/client_test.go`: add request-path, header, `Content-Type`, and error-mapping coverage
- `test/integration/xero_invoices_integration_test.go`: add refresh + tenant + file-output coverage
- `test/output/json_contract_test.go`: add envelope and quiet assertions for `InvoicePDFResult`
- `docs/commands/invoices.md` and `README.md`: document the new workflow and binary-output caveats

Suggested v1 command contract:

```bash
# docs/commands/invoices.md
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output -
xero invoices pdf --invoice-id 220ddca8-3144-4085-9a88-2d72c5133734 --output invoice.pdf --json
```

## Scope Boundaries

In scope for v1:

- retrieve one invoice PDF by invoice ID
- support explicit file output and explicit stdout streaming
- preserve existing auth, tenant, and typed-error behavior
- add docs and tests that explain binary-output behavior clearly

Out of scope for v1:

- invoice lookup by invoice number
- automatic file naming or output directories
- opening the PDF in a viewer after download
- batch invoice downloads or zip output
- quote and credit-note PDF commands
- base64 payloads in JSON output

## Sources & References

### Internal References

- current invoice command namespace and validation helpers: `internal/commands/invoices_list.go:33`
- existing online-invoice subcommand pattern: `internal/commands/invoices_online_url.go:13`
- runtime JSON vs human output split: `internal/commands/root.go:195`
- JSON envelope contract: `internal/output/json.go:13`
- current human invoice and online-url writers: `internal/output/human.go:12`
- invoice command docs: `docs/commands/invoices.md:1`
- top-level CLI examples and output-mode docs: `README.md:3`
- testing guidance: `docs/development/testing.md:1`
- related shipped plan to mirror structurally: `docs/plans/2026-03-11-feat-add-online-invoice-url-command-plan.md:10`

### External References

- Xero SDK docs for invoice PDF retrieval: `https://xeroapi.github.io/xero-node/accounting/index.html#api-Accounting-getInvoiceAsPdf`
- Xero Accounting OpenAPI showing `/Invoices/{InvoiceID}/pdf` and `application/pdf`: `https://raw.githubusercontent.com/XeroAPI/Xero-OpenAPI/master/xero_accounting.yaml`
- Xero invoice API documentation describing PDF retrieval via `Accept: application/pdf`: `https://developer.xero.com/documentation/api/accounting/invoices`
- Command Line Interface Guidelines on stdout/stderr, machine-readable output, and `--output`: `https://clig.dev/`

### Related Work

- related prior plan: `docs/plans/2026-03-11-feat-add-online-invoice-url-command-plan.md:1`
- no matching brainstorm document found under `docs/brainstorms/`
- no project-tracker issue or PR reference discovered during local research

### AI-Assisted Research Notes

- local repo research confirmed this project is a Go Cobra/Viper CLI rather than an MCP server, so the feature should be planned as a subcommand instead of a `server.tool(...)` addition
- adjacent institutional prior art strongly favors reusing the command/runtime/client split and preserving the JSON envelope contract wherever possible
- external research identified a documentation ambiguity between `/Invoices/{InvoiceID}/pdf` and base invoice retrieval with `Accept: application/pdf`; the implementation should verify the wire contract and encode the decision in client tests
