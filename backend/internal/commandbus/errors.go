package commandbus

import (
	"errors"
	"fmt"
)

// Code is the machine-readable category of a command rejection or failure.
// The C0 contract (docs/architecture/enterprise-foundation-v1.0.0.md §5, D3)
// requires every rejection class to be distinguishable: unregistered command,
// missing permission, invalid payload, optimistic-concurrency conflict and
// domain-level rejection each carry their own code.
type Code string

const (
	// CodeUnknownCommand — no handler is registered for the command contract.
	CodeUnknownCommand Code = "unknown_command"
	// CodePermissionDenied — the actor kind (or the fine-grained policy)
	// forbids executing this command.
	CodePermissionDenied Code = "permission_denied"
	// CodeInvalidPayload — the envelope or the command payload is malformed.
	CodeInvalidPayload Code = "invalid_payload"
	// CodeVersionConflict — expectedVersion did not match the stored version
	// (optimistic concurrency rejection).
	CodeVersionConflict Code = "version_conflict"
	// CodeDomainRejected — the owning domain rejected the change for a
	// business rule (illegal transition, unknown target, duplicate link, ...).
	CodeDomainRejected Code = "domain_rejected"
	// CodeInternal — unexpected infrastructure failure (I/O, commit error).
	CodeInternal Code = "internal"
)

// Error is the structured error every gateway and handler failure produces.
// It wraps the underlying cause (when any) so callers can still match domain
// sentinels via errors.Is, while CodeOf exposes the contract category.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
	cause   error
}

func (e *Error) Error() string {
	return fmt.Sprintf("commandbus: [%s] %s", e.Code, e.Message)
}

// Unwrap exposes the wrapped cause for errors.Is/errors.As matching.
func (e *Error) Unwrap() error { return e.cause }

// NewError builds a structured command error with the given code.
func NewError(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// WrapError builds a structured command error that keeps err as its cause.
func WrapError(code Code, err error, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...), cause: err}
}

// CodeOf extracts the command error code from err. ok=false when err is not
// a commandbus error.
func CodeOf(err error) (Code, bool) {
	var e *Error
	if !errors.As(err, &e) {
		return "", false
	}
	return e.Code, true
}

// IsCode reports whether err is a command error carrying code.
func IsCode(err error, code Code) bool {
	c, ok := CodeOf(err)
	return ok && c == code
}

// classify returns err as a *Error, wrapping non-command errors as
// CodeInternal so every handler failure is auditable under a stable code.
func classify(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return WrapError(CodeInternal, err, "%v", err)
}
