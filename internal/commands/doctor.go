package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/inscoder/xero-cli/internal/auth"
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
				checkFile("auth", rt.Settings.AuthFilePath),
				checkFile("tokens", rt.Tokens.FallbackPath()),
				checkBrowserCommand(rt.LookPath, rt.Settings.OpenCommand),
				checkValue("client-id", rt.Settings.ClientID != "", "OAuth client ID available"),
				checkValue("client-secret", rt.Settings.ClientSecret != "", "OAuth client secret available"),
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

func checkBrowserCommand(lookPath func(string) error, configured string) output.DoctorCheck {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		return checkCommand(lookPath, "browser", configured)
	}
	commands := auth.DefaultBrowserCommands()
	if len(commands) == 0 {
		return output.DoctorCheck{Name: "browser", Status: "warn", Detail: "not configured"}
	}
	for _, command := range commands {
		if err := lookPath(command); err == nil {
			return output.DoctorCheck{Name: "browser", Status: "ok", Detail: command}
		}
	}
	return output.DoctorCheck{Name: "browser", Status: "warn", Detail: fmt.Sprintf("none found: %s", strings.Join(commands, ", "))}
}

func checkValue(name string, ok bool, detail string) output.DoctorCheck {
	status := "warn"
	if ok {
		status = "ok"
	}
	return output.DoctorCheck{Name: name, Status: status, Detail: detail}
}
