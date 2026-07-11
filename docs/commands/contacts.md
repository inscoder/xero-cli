# `xero contacts`

The Contacts command requires an explicit `list`, `create`, or `update` subcommand.

## List contacts

```bash
xero contacts list
xero contacts list --search "Acme"
xero contacts list --contact-id 00000000-0000-0000-0000-000000000001 --include-archived --json
xero contacts list --page 2 --page-size 100 --summary-only
```

List filters include repeatable or comma-separated `--contact-id`, `--search`, `--page`, `--page-size`, `--include-archived`, `--summary-only`, `--since YYYY-MM-DD`, `--where`, and validated `--order "Field ASC|DESC"`. Combined filters use Xero's intersection semantics. When `--summary-only` is enabled, do not filter on fields omitted by Xero's summary projection. `--since` is sent as `If-Modified-Since`; an unchanged response is reported as an empty successful result.

The human table and structured contact model intentionally omit bank-account, tax-identifier, balance, and attachment data.

## Create a contact

Use either scalar flags or one strict JSON file, never both:

```bash
xero contacts create --name "Acme Corp" --email acme@example.com --phone "+1234567890"
xero contacts create --file docs/examples/contact-create.json
printf '%s\n' '{"name":"Acme Corp","emailAddress":"acme@example.com"}' | xero contacts create --file - --json
```

Flag mode requires `--name`; `--email` and `--phone` are optional. File mode accepts `name`, `contactNumber`, `accountNumber`, `firstName`, `lastName`, `companyNumber`, `emailAddress`, and `phone`. A supplied phone becomes one Xero `DEFAULT` phone. The input is limited to one 1 MiB camelCase UTF-8 object and rejects arrays, wrappers, unknown or duplicate fields, `null`, a BOM, and trailing data before authentication or network access.

`--idempotency-key` accepts a caller-owned 1–128 byte key for an exact retry. The CLI generates and reports a key when it is omitted.

## Update or archive a contact

Use either changed scalar flags or a strict partial JSON file:

```bash
xero contacts update \
  --contact-id 00000000-0000-0000-0000-000000000001 \
  --name "Acme Corporation" \
  --email new@acme.com

xero contacts update --file docs/examples/contact-update.json

xero contacts update \
  --contact-id 00000000-0000-0000-0000-000000000001 \
  --status ARCHIVED \
  --confirm-archive
```

Flag mode requires `--contact-id` and at least one changed data flag. File mode requires `contactId`; it cannot be mixed with scalar data flags. Supported partial fields are `name`, `contactNumber`, `accountNumber`, `firstName`, `lastName`, `companyNumber`, `emailAddress`, and `contactStatus`. Empty optional scalar values remain present so Xero can clear them. Phone updates and nested contact fields are deliberately excluded in this version.

Only `ACTIVE` and `ARCHIVED` status changes are allowed. Archiving must be a status-only update and requires `--confirm-archive`. Reactivating with `ACTIVE` does not need confirmation.

## OAuth and uncertain outcomes

Listing requires `accounting.contacts.read` or `accounting.contacts`. Create, update, and archive require `accounting.contacts`. Re-run `xero login -p <profile>` after changing scopes; a forbidden response is `PermissionDenied` (exit code 21).

The CLI does not automatically retry contact mutations. If the network or response makes the final state ambiguous, the command exits with `MutationUncertain` (exit code 20), reports the effective idempotency key, and prints a read-only recovery command. Verify remote state before retrying. Contact names are not unique, so exact read-back should use `--contact-id` whenever one is known.
