# Contact sandbox checklist

Use this only with the Xero Demo Company connected to profile `demo`. Every Xero API command below explicitly passes `-p demo --no-browser`; do not rely on the default profile or `XERO_PROFILE`.

## Guardrails

- Stop before mutation if `demo` is absent, points at the wrong organisation, cannot list contacts, or lacks `accounting.contacts`.
- Use only synthetic names, an `example.invalid` email address, and a harmless phone number.
- Keep ContactIDs and response payloads in shell variables or `/tmp`; never commit them.
- Use caller-supplied idempotency keys. Never retry `MutationUncertain` until its read-only recovery command has verified remote state.
- Finish by archiving the created contact and confirming archived read-back.

## 1. Prepare and prove read access

```bash
go build -o /tmp/xero ./cmd/xero
/tmp/xero profile list --json
/tmp/xero contacts list -p demo --no-browser --page 1 --page-size 1 --json

RUN_ID="$(date -u +%Y%m%dT%H%M%SZ)"
CREATE_KEY="xero-cli-demo-contact-create-${RUN_ID}"
```

Confirm the profile list contains `demo` and the contact request succeeds before continuing.

## 2. Create one synthetic contact

```bash
CREATE_OUTPUT="$(/tmp/xero contacts create \
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

Verify `operation: created`, `resource: contact`, status `ACTIVE`, and the supplied key without printing or saving the full response.

## 3. Verify search and exact identity

```bash
/tmp/xero contacts list \
  -p demo \
  --no-browser \
  --search "xero-cli demo ${RUN_ID}" \
  --page 1 \
  --json

/tmp/xero contacts list \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --json
```

Search must include the synthetic contact. Exact-ID output must contain only the expected ContactID with status `ACTIVE`.

## 4. Exercise flag and file updates

```bash
UPDATE_KEY="xero-cli-demo-contact-update-flags-${RUN_ID}"

/tmp/xero contacts update \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --name "xero-cli demo updated ${RUN_ID}" \
  --email "xero-cli-updated-${RUN_ID}@example.invalid" \
  --idempotency-key "${UPDATE_KEY}" \
  --json

/tmp/xero contacts list \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --json

UPDATE_FILE="/tmp/xero-contact-update-${RUN_ID}.json"
jq -n \
  --arg contactId "${CONTACT_ID}" \
  --arg contactNumber "CLI-${RUN_ID}" \
  '{contactId: $contactId, contactNumber: $contactNumber}' \
  > "${UPDATE_FILE}"

/tmp/xero contacts update \
  -p demo \
  --no-browser \
  --file "${UPDATE_FILE}" \
  --idempotency-key "xero-cli-demo-contact-update-file-${RUN_ID}" \
  --json

/tmp/xero contacts list \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --json
```

Both updates must return the same ContactID. The file update must change `contactNumber` without clearing the omitted name or email.

## 5. Prove archive protection and clean up

The first command must fail locally with `ValidationError` exit code 12:

```bash
/tmp/xero contacts update \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --status ARCHIVED \
  --idempotency-key "xero-cli-demo-contact-archive-refused-${RUN_ID}" \
  --json

/tmp/xero contacts list \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --json

ARCHIVE_KEY="xero-cli-demo-contact-archive-${RUN_ID}"

/tmp/xero contacts update \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --status ARCHIVED \
  --confirm-archive \
  --idempotency-key "${ARCHIVE_KEY}" \
  --json

/tmp/xero contacts list \
  -p demo \
  --no-browser \
  --contact-id "${CONTACT_ID}" \
  --include-archived \
  --json
```

The refusal read-back must still show `ACTIVE`. The confirmed mutation and final read-back must both show the same ContactID with status `ARCHIVED`; this is the cleanup state because Xero has no contact delete endpoint.

## 6. Remove temporary data

```bash
rm -f "${UPDATE_FILE}"
unset CREATE_OUTPUT CONTACT_ID CREATE_KEY UPDATE_KEY ARCHIVE_KEY RUN_ID UPDATE_FILE
```

## Mutation uncertainty

For uncertain create, run the reported search recovery with `-p demo --no-browser --include-archived`. For uncertain update/archive, use exact-ID read-back with those flags. Reuse the same idempotency key only when verification shows the exact request did not take effect.
