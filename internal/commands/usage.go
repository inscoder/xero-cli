package commands

import "github.com/spf13/cobra"

func exactArgsWithUsage(n int) cobra.PositionalArgs {
	validator := cobra.ExactArgs(n)
	return func(cmd *cobra.Command, args []string) error {
		if err := validator(cmd, args); err != nil {
			_ = cmd.Usage()
			return err
		}
		return nil
	}
}
