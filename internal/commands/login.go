package commands

import (
	"fmt"
	"io"

	"github.com/inscoder/xero-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newLoginCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to Xero via browser",
		RunE: func(cmd *cobra.Command, args []string) error {
			applyClientIDFlag(cmd, v)
			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			ctx, cancel := rt.Context()
			defer cancel()
			result, err := rt.Auth.Login(ctx)
			if err != nil {
				return err
			}
			data := map[string]any{
				"profile":       rt.Settings.ProfileName,
				"authMode":      result.Token.AuthMode,
				"generatedAt":   result.Token.GeneratedAt,
				"expiresAt":     result.Token.ExpiresAt,
				"defaultTenant": result.Default,
				"tenantCount":   len(result.Tenants),
			}
			breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: "xero invoices --json"}}
			return rt.WriteData(data, fmt.Sprintf("Logged in to %d tenant(s)", len(result.Tenants)), breadcrumbs, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Authenticated profile %s with Xero. Tenant: %s (%s)\n", rt.Settings.ProfileName, result.Default.Name, result.Default.ID)
				return err
			})
		},
	}
	cmd.Flags().String("scope", "", "Override OAuth scopes (space-separated; required OAuth scopes are auto-prepended)")
	addClientIDFlag(cmd)
	mustBind(v, "auth.scopes", cmd.Flags().Lookup("scope"))
	return cmd
}
