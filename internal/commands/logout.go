package commands

import (
	"fmt"
	"io"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newLogoutCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out from Xero for the active profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			if rt.Settings.ProfileName == "" {
				return clierrors.New(clierrors.KindValidation, "no profile configured; run `xero profile add <name>` first")
			}
			if err := rt.Tokens.Clear(); err != nil {
				return err
			}
			return rt.WriteData(map[string]any{"ok": true, "profile": rt.Settings.ProfileName}, "Logged out", nil, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Logged out from profile %q.\n", rt.Settings.ProfileName)
				return err
			})
		},
	}
}
