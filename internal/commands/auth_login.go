package commands

import (
	"fmt"
	"io"

	"github.com/inscoder/xero-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newAuthCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{Use: "auth", Short: "Authentication commands"}
	cmd.AddCommand(newAuthLoginCommand(deps, v), newAuthStatusCommand(deps, v), newAuthLogoutCommand(deps, v))
	return cmd
}

func newAuthLoginCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Xero in the browser",
		RunE: func(cmd *cobra.Command, args []string) error {
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
				"authMode":      result.Token.AuthMode,
				"generatedAt":   result.Token.GeneratedAt,
				"expiresAt":     result.Token.ExpiresAt,
				"defaultTenant": result.Default,
				"tenantCount":   len(result.Tenants),
			}
			breadcrumbs := []output.Breadcrumb{{Action: "show", Cmd: "xero auth status --json"}, {Action: "show", Cmd: "xero invoices --json"}}
			return rt.WriteData(data, fmt.Sprintf("Logged in to %d tenant(s)", len(result.Tenants)), breadcrumbs, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Authenticated with Xero. Default tenant: %s (%s)\n", result.Default.Name, result.Default.ID)
				return err
			})
		},
	}
	return cmd
}
