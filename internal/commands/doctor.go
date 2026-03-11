package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/inscoder/xero-cli/internal/output"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func newDoctorCommand(deps Dependencies, v *viper.Viper) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Validate local config and auth prerequisites",
		RunE: func(cmd *cobra.Command, args []string) error {
			rt, err := loadRuntime(deps, v)
			if err != nil {
				return err
			}
			checks := []output.DoctorCheck{
				checkFile("config", rt.Settings.ConfigFilePath),
				checkFile("tokens", rt.Tokens.FallbackPath()),
				checkCommand(rt.LookPath, "browser", firstNonEmpty(rt.Settings.OpenCommand, defaultBrowserCommand())),
				checkValue("client-id", rt.Settings.ClientID != "", "XERO_AUTH_CLIENT_ID configured"),
				checkValue("default-tenant", rt.Settings.DefaultTenantID != "", firstNonEmpty(rt.Settings.DefaultTenantName, rt.Settings.DefaultTenantID)),
				checkValue("token-storage", true, rt.Tokens.StorageMode()),
				checkValue("known-tenants", len(rt.SessionMeta.KnownTenants) > 0, fmt.Sprintf("%d discovered", len(rt.SessionMeta.KnownTenants))),
			}
			return rt.WriteData(checks, "Doctor checks complete", nil, func(w io.Writer) error {
				return output.WriteDoctor(w, checks)
			})
		},
	}
}

func checkFile(name, path string) output.DoctorCheck {
	if _, err := os.Stat(path); err == nil {
		return output.DoctorCheck{Name: name, Status: "ok", Detail: path}
	}
	return output.DoctorCheck{Name: name, Status: "warn", Detail: path}
}

func checkCommand(lookPath func(string) error, name, command string) output.DoctorCheck {
	if command == "" {
		return output.DoctorCheck{Name: name, Status: "warn", Detail: "not configured"}
	}
	if err := lookPath(command); err != nil {
		return output.DoctorCheck{Name: name, Status: "warn", Detail: err.Error()}
	}
	return output.DoctorCheck{Name: name, Status: "ok", Detail: command}
}

func checkValue(name string, ok bool, detail string) output.DoctorCheck {
	status := "warn"
	if ok {
		status = "ok"
	}
	return output.DoctorCheck{Name: name, Status: status, Detail: detail}
}

func defaultBrowserCommand() string {
	return "open"
}
