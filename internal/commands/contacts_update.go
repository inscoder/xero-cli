package commands

import (
	"fmt"
	"io"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/output"
	"github.com/inscoder/xero-cli/internal/xeroapi"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newContactsUpdateCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	var filePath string
	var contactID string
	var name string
	var email string
	var contactNumber string
	var accountNumber string
	var firstName string
	var lastName string
	var companyNumber string
	var status string
	var confirmArchive bool
	var idempotencyKey string

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update one Xero contact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dataFlags := []string{"contact-id", "name", "email", "contact-number", "account-number", "first-name", "last-name", "company-number", "status"}
			fileMode := cmd.Flags().Changed("file")
			if fileMode && anyFlagChanged(cmd, dataFlags...) {
				return clierrors.New(clierrors.KindValidation, "--file cannot be combined with contact data flags")
			}

			var input contactUpdateInput
			if fileMode {
				if err := decodeJSONInput(filePath, deps.IO.In, deps.IsTerminal(0), &input); err != nil {
					return err
				}
			} else {
				if !cmd.Flags().Changed("contact-id") {
					return clierrors.New(clierrors.KindValidation, "--contact-id is required when --file is not used")
				}
				input.ContactID = contactID
				if cmd.Flags().Changed("name") {
					input.Name = contactStringPointer(name)
				}
				if cmd.Flags().Changed("email") {
					input.EmailAddress = contactStringPointer(email)
				}
				if cmd.Flags().Changed("contact-number") {
					input.ContactNumber = contactStringPointer(contactNumber)
				}
				if cmd.Flags().Changed("account-number") {
					input.AccountNumber = contactStringPointer(accountNumber)
				}
				if cmd.Flags().Changed("first-name") {
					input.FirstName = contactStringPointer(firstName)
				}
				if cmd.Flags().Changed("last-name") {
					input.LastName = contactStringPointer(lastName)
				}
				if cmd.Flags().Changed("company-number") {
					input.CompanyNumber = contactStringPointer(companyNumber)
				}
				if cmd.Flags().Changed("status") {
					input.ContactStatus = contactStringPointer(status)
				}
			}
			if err := validateContactUpdateInput(&input, confirmArchive); err != nil {
				return err
			}
			effectiveKey, err := resolveIdempotencyKey(idempotencyKey, cmd.Flags().Changed("idempotency-key"))
			if err != nil {
				return err
			}

			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			if rt.Contacts == nil {
				return clierrors.New(clierrors.KindInternal, "contact client is not configured")
			}
			token, err := rt.LoadToken()
			if err != nil {
				return err
			}
			token, err = rt.EnsureToken(token)
			if err != nil {
				return err
			}
			tenant, err := rt.Tenants.ResolveTokenTenant(token)
			if err != nil {
				return err
			}

			ctx, cancel := rt.Context()
			defer cancel()
			result, err := rt.Contacts.UpdateContact(ctx, token, xeroapi.UpdateContactRequest{
				TenantID:       tenant.ID,
				ContactID:      input.ContactID,
				IdempotencyKey: effectiveKey,
				Contact:        contactUpdateWrite(input),
			})
			if err != nil {
				return err
			}
			breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: fmt.Sprintf("xero contacts list --contact-id %s --include-archived --json", result.ContactID)}}
			return rt.WriteData(result, "contact updated", breadcrumbs, func(writer io.Writer) error {
				return output.WriteContactMutation(writer, result)
			})
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "JSON input file path or - for stdin")
	cmd.Flags().StringVar(&contactID, "contact-id", "", "contact ID")
	cmd.Flags().StringVar(&name, "name", "", "contact name")
	cmd.Flags().StringVar(&email, "email", "", "contact email address")
	cmd.Flags().StringVar(&contactNumber, "contact-number", "", "external contact number")
	cmd.Flags().StringVar(&accountNumber, "account-number", "", "contact account number")
	cmd.Flags().StringVar(&firstName, "first-name", "", "contact first name")
	cmd.Flags().StringVar(&lastName, "last-name", "", "contact last name")
	cmd.Flags().StringVar(&companyNumber, "company-number", "", "contact company number")
	cmd.Flags().StringVar(&status, "status", "", "contact status (ACTIVE or ARCHIVED)")
	cmd.Flags().BoolVar(&confirmArchive, "confirm-archive", false, "confirm a status-only archive update")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "idempotency key for an exact retry (1-128 bytes)")
	return cmd
}

func contactUpdateWrite(input contactUpdateInput) xeroapi.ContactWrite {
	return xeroapi.ContactWrite{
		ContactID:     input.ContactID,
		Name:          input.Name,
		ContactNumber: input.ContactNumber,
		AccountNumber: input.AccountNumber,
		FirstName:     input.FirstName,
		LastName:      input.LastName,
		CompanyNumber: input.CompanyNumber,
		EmailAddress:  input.EmailAddress,
		ContactStatus: input.ContactStatus,
	}
}
