package commands

import (
	"fmt"
	"io"

	"github.com/inscoder/xero-cli/internal/output"
	appversion "github.com/inscoder/xero-cli/internal/version"
	"github.com/spf13/cobra"
)

func newVersionCommand(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show CLI version information",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			info := appversion.Current()
			if deps.Version != "" {
				info.Version = deps.Version
			}

			quiet, _ := cmd.Flags().GetBool("quiet")
			jsonOutput, _ := cmd.Flags().GetBool("json")
			if quiet || jsonOutput {
				return output.WriteJSON(deps.IO.Out, info, "Version information", nil, quiet)
			}

			return writeVersionHuman(deps.IO.Out, info)
		},
	}
}

func writeVersionHuman(w io.Writer, info appversion.Info) error {
	if _, err := fmt.Fprintf(w, "xero %s\n", info.Version); err != nil {
		return err
	}
	if info.Commit != "" && info.Commit != "none" {
		if _, err := fmt.Fprintf(w, "commit: %s\n", info.Commit); err != nil {
			return err
		}
	}
	if info.Date != "" && info.Date != "unknown" {
		if _, err := fmt.Fprintf(w, "built: %s\n", info.Date); err != nil {
			return err
		}
	}
	return nil
}
