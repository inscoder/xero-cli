package commands

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strings"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newProfileCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	cmd := &cobra.Command{Use: "profile", Short: "Manage Xero profiles"}
	cmd.AddCommand(newProfileAddCommand(deps, v), newProfileListCommand(deps, v), newProfileSetDefaultCommand(deps, v), newProfileRemoveCommand(deps, v))
	return cmd
}

func newProfileAddCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	var clientID string
	var force bool
	cmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a Xero profile",
		Args:  exactArgsWithUsage(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			clientID = strings.TrimSpace(clientID)
			if clientID == "" {
				if !rt.Settings.Interactive {
					return clierrors.New(clierrors.KindValidation, "--client-id is required in non-interactive mode")
				}
				fmt.Fprint(rt.IO.ErrOut, "Xero Client ID: ")
				line, readErr := bufio.NewReader(rt.IO.In).ReadString('\n')
				if readErr != nil {
					return clierrors.Wrap(clierrors.KindValidation, "read client ID", readErr)
				}
				clientID = strings.TrimSpace(line)
			}
			if err := rt.Config.AddProfile(args[0], clientID, force); err != nil {
				return err
			}
			data := map[string]any{"name": args[0], "clientId": clientID}
			return rt.WriteData(data, "Profile added", nil, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Profile %q added. Run `xero login -p %s` to authenticate.\n", args[0], args[0])
				return err
			})
		},
	}
	cmd.Flags().StringVar(&clientID, "client-id", "", "Xero OAuth client ID")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "overwrite an existing profile")
	return cmd
}

func newProfileListCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Xero profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			cfg := rt.Config.LoadedConfig()
			type profileRow struct {
				Name      string `json:"name"`
				ClientID  string `json:"clientId"`
				IsDefault bool   `json:"default"`
			}
			rows := make([]profileRow, 0, len(cfg.Profiles))
			for name, profile := range cfg.Profiles {
				rows = append(rows, profileRow{Name: name, ClientID: profile.ClientID, IsDefault: name == cfg.DefaultProfile})
			}
			sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
			return rt.WriteData(rows, fmt.Sprintf("%d profile(s)", len(rows)), nil, func(w io.Writer) error {
				if len(rows) == 0 {
					_, err := fmt.Fprintln(w, "No profiles configured. Run `xero profile add <name>` to add one.")
					return err
				}
				for _, row := range rows {
					marker := ""
					if row.IsDefault {
						marker = " (default)"
					}
					if _, err := fmt.Fprintf(w, "%s%s\n  Client ID: %s\n", row.Name, marker, maskClientID(row.ClientID)); err != nil {
						return err
					}
				}
				return nil
			})
		},
	}
}

func newProfileSetDefaultCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "set-default <name>",
		Short: "Set the default Xero profile",
		Args:  exactArgsWithUsage(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			if err := rt.Config.SetDefaultProfile(args[0]); err != nil {
				return err
			}
			return rt.WriteData(map[string]any{"defaultProfile": args[0]}, "Default profile set", nil, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Default profile set to %q.\n", args[0])
				return err
			})
		},
	}
}

func newProfileRemoveCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a Xero profile",
		Args:  exactArgsWithUsage(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			if err := rt.Config.RemoveProfile(args[0]); err != nil {
				return err
			}
			return rt.WriteData(map[string]any{"removed": args[0]}, "Profile removed", nil, func(w io.Writer) error {
				_, err := fmt.Fprintf(w, "Profile %q removed.\n", args[0])
				return err
			})
		},
	}
}

func maskClientID(value string) string {
	if len(value) <= 8 {
		return value
	}
	return value[:8] + "..."
}
