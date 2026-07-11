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

func newContactsCreateCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	var filePath string
	var name string
	var email string
	var phone string
	var idempotencyKey string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create one Xero contact",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fileMode := cmd.Flags().Changed("file")
			if fileMode && anyFlagChanged(cmd, "name", "email", "phone") {
				return clierrors.New(clierrors.KindValidation, "--file cannot be combined with contact data flags")
			}

			var input contactCreateInput
			if fileMode {
				if err := decodeJSONInput(filePath, deps.IO.In, deps.IsTerminal(0), &input); err != nil {
					return err
				}
			} else {
				if !cmd.Flags().Changed("name") {
					return clierrors.New(clierrors.KindValidation, "--name is required when --file is not used")
				}
				input.Name = name
				if cmd.Flags().Changed("email") {
					input.EmailAddress = contactStringPointer(email)
				}
				if cmd.Flags().Changed("phone") {
					input.Phone = contactStringPointer(phone)
				}
			}
			if err := validateContactCreateInput(input); err != nil {
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
			result, err := rt.Contacts.CreateContact(ctx, token, xeroapi.CreateContactRequest{
				TenantID:       tenant.ID,
				IdempotencyKey: effectiveKey,
				Contact:        contactCreateWrite(input),
			})
			if err != nil {
				return err
			}
			breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: fmt.Sprintf("xero contacts list --contact-id %s --include-archived --json", result.ContactID)}}
			return rt.WriteData(result, "contact created", breadcrumbs, func(writer io.Writer) error {
				return output.WriteContactMutation(writer, result)
			})
		},
	}
	cmd.Flags().StringVar(&filePath, "file", "", "JSON input file path or - for stdin")
	cmd.Flags().StringVar(&name, "name", "", "contact name")
	cmd.Flags().StringVar(&email, "email", "", "contact email address")
	cmd.Flags().StringVar(&phone, "phone", "", "default contact phone number")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "idempotency key for an exact retry (1-128 bytes)")
	return cmd
}

func contactCreateWrite(input contactCreateInput) xeroapi.ContactWrite {
	write := xeroapi.ContactWrite{
		Name:          &input.Name,
		ContactNumber: input.ContactNumber,
		AccountNumber: input.AccountNumber,
		FirstName:     input.FirstName,
		LastName:      input.LastName,
		CompanyNumber: input.CompanyNumber,
		EmailAddress:  input.EmailAddress,
	}
	if input.Phone != nil {
		phones := []xeroapi.ContactWritePhone{{PhoneType: "DEFAULT", PhoneNumber: *input.Phone}}
		write.Phones = &phones
	}
	return write
}

func anyFlagChanged(cmd *cobra.Command, names ...string) bool {
	for _, name := range names {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}
