package commands

import (
	"fmt"
	"regexp"
	"strings"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

var (
	uuidPattern       = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	orderFieldPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9.]*$`)
)

func normalizeUUIDs(values []string, flagName string) ([]string, error) {
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		candidate := strings.TrimSpace(value)
		if candidate == "" {
			return nil, clierrors.New(clierrors.KindValidation, fmt.Sprintf("%s values must not be empty", flagName))
		}
		if !uuidPattern.MatchString(candidate) {
			return nil, clierrors.New(clierrors.KindValidation, fmt.Sprintf("%s must be a valid UUID", flagName))
		}
		normalized = append(normalized, strings.ToLower(candidate))
	}
	return normalized, nil
}

func normalizeOrderClause(value string, changed bool, defaultValue string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		if changed {
			return "", clierrors.New(clierrors.KindValidation, "--order must not be empty")
		}
		return defaultValue, nil
	}
	parts := strings.Fields(trimmed)
	if len(parts) != 2 || !orderFieldPattern.MatchString(parts[0]) {
		return "", clierrors.New(clierrors.KindValidation, "--order must use '<Field> <ASC|DESC>'")
	}
	direction := strings.ToUpper(parts[1])
	if direction != "ASC" && direction != "DESC" {
		return "", clierrors.New(clierrors.KindValidation, "--order must use '<Field> <ASC|DESC>'")
	}
	return parts[0] + " " + direction, nil
}
