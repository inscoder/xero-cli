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
	KindMutationUncertain       Kind = "MutationUncertainError"
	KindPermissionDenied        Kind = "PermissionDeniedError"
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
	ExitUncertain  = 20
	ExitPermission = 21
)

// Metadata contains optional, structured details that callers can use to
// recover from API failures without parsing the human-readable message.
type Metadata struct {
	ValidationErrors  []string `json:"validationErrors,omitempty"`
	MayHaveSucceeded  bool     `json:"mayHaveSucceeded,omitempty"`
	Operation         string   `json:"operation,omitempty"`
	Resource          string   `json:"resource,omitempty"`
	TenantID          string   `json:"tenantId,omitempty"`
	InvoiceID         string   `json:"invoiceId,omitempty"`
	FileName          string   `json:"fileName,omitempty"`
	IdempotencyKey    string   `json:"idempotencyKey,omitempty"`
	RetryAfterSeconds int      `json:"retryAfterSeconds,omitempty"`
	RecoveryCommand   string   `json:"recoveryCommand,omitempty"`
}

type CLIError struct {
	Kind     Kind
	Message  string
	Cause    error
	Metadata Metadata
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

func NewWithMetadata(kind Kind, message string, metadata Metadata) error {
	return &CLIError{Kind: kind, Message: message, Metadata: metadata}
}

func Wrap(kind Kind, message string, cause error) error {
	return &CLIError{Kind: kind, Message: message, Cause: cause}
}

func WrapWithMetadata(kind Kind, message string, cause error, metadata Metadata) error {
	return &CLIError{Kind: kind, Message: message, Cause: cause, Metadata: metadata}
}

func KindOf(err error) Kind {
	var cliErr *CLIError
	if stderrors.As(err, &cliErr) {
		return cliErr.Kind
	}
	return KindInternal
}

func MetadataOf(err error) Metadata {
	var cliErr *CLIError
	if stderrors.As(err, &cliErr) {
		return cliErr.Metadata
	}
	return Metadata{}
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
	case KindMutationUncertain:
		return ExitUncertain
	case KindPermissionDenied:
		return ExitPermission
	default:
		return ExitInternal
	}
}
