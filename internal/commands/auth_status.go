package commands

import (
	"errors"
	"io"

	"github.com/inscoder/xero-cli/internal/auth"
	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/inscoder/xero-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newAuthStatusCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current auth status",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			token, err := rt.Tokens.Load()
			authenticated := err == nil
			if err != nil && !errors.Is(err, auth.ErrTokenNotFound) {
				return clierrors.Wrap(clierrors.KindConfigCorrupted, "read auth status", err)
			}
			refreshNeeded := false
			if err == nil {
				refreshNeeded = auth.ShouldRefresh(token, rt.Now(), rt.Settings.RefreshAfter)
			}
			data := map[string]any{
				"authenticated":     authenticated,
				"authMode":          rt.SessionMeta.AuthMode,
				"generatedAt":       rt.SessionMeta.GeneratedAt,
				"expiresAt":         rt.SessionMeta.ExpiresAt,
				"refreshNeeded":     refreshNeeded,
				"defaultTenantId":   rt.Settings.DefaultTenantID,
				"defaultTenantName": rt.Settings.DefaultTenantName,
				"tenantCount":       len(rt.SessionMeta.KnownTenants),
				"storageMode":       rt.SessionMeta.StorageMode,
			}
			return rt.WriteData(data, "Auth status", nil, func(w io.Writer) error {
				return output.WriteStatus(w, authenticated, rt.SessionMeta, rt.Settings.DefaultTenantID, rt.Settings.DefaultTenantName, refreshNeeded)
			})
		},
	}
}
