package errors

import (
	stderrors "errors"
	"fmt"
)

type Kind string

const (
	KindAuthRequired            Kind = "AuthRequiredError"
	KindTokenRefreshFailed      Kind = "TokenRefreshFailedError"
	KindTenantSelectionRequired Kind = "TenantSelectionRequiredError"
	KindConfigCorrupted         Kind = "ConfigCorruptedError"
	KindXeroRequest             Kind = "XeroRequestError"
	KindXeroAPI                 Kind = "XeroApiError"
	KindNetwork                 Kind = "NetworkError"
	KindRateLimit               Kind = "RateLimitError"
	KindValidation              Kind = "ValidationError"
	KindInternal                Kind = "InternalError"
)

const (
	ExitSuccess    = 0
	ExitAuth       = 10
	ExitConfig     = 11
	ExitValidation = 12
	ExitNetwork    = 13
	ExitAPI        = 14
	ExitRateLimit  = 15
	ExitRequest    = 16
	ExitInternal   = 17
	ExitTenant     = 18
	ExitRefresh    = 19
)

type CLIError struct {
	Kind    Kind
	Message string
	Cause   error
}

func (e *CLIError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return e.Message
	}
	return fmt.Sprintf("%s: %v", e.Message, e.Cause)
}

func (e *CLIError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func New(kind Kind, message string) error {
	return &CLIError{Kind: kind, Message: message}
}

func Wrap(kind Kind, message string, cause error) error {
	return &CLIError{Kind: kind, Message: message, Cause: cause}
}

func KindOf(err error) Kind {
	var cliErr *CLIError
	if stderrors.As(err, &cliErr) {
		return cliErr.Kind
	}
	return KindInternal
}

func ExitCode(err error) int {
	switch KindOf(err) {
	case KindAuthRequired:
		return ExitAuth
	case KindTokenRefreshFailed:
		return ExitRefresh
	case KindTenantSelectionRequired:
		return ExitTenant
	case KindConfigCorrupted:
		return ExitConfig
	case KindValidation:
		return ExitValidation
	case KindNetwork:
		return ExitNetwork
	case KindXeroAPI:
		return ExitAPI
	case KindRateLimit:
		return ExitRateLimit
	case KindXeroRequest:
		return ExitRequest
	default:
		return ExitInternal
	}
}
