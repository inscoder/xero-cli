package commands

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newAuthLogoutCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Clear saved Xero session state",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			if err := rt.Tokens.Clear(); err != nil {
				return err
			}
			if err := rt.Session.Clear(); err != nil {
				return err
			}
			if err := rt.Config.ClearDefaultTenant(); err != nil {
				return err
			}
			return rt.WriteData(map[string]any{"ok": true}, "Logged out", nil, func(w io.Writer) error {
				_, err := fmt.Fprintln(w, "Cleared saved Xero session state.")
				return err
			})
		},
	}
}
