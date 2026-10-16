package core

import (
	"errors"
	"fmt"
)

// UserError is an error whose message is meant for end users: validation
// problems, missing records, or problems with a remote resource the user
// supplied (a feed URL, an uploaded file). The web layer shows its message;
// every other error is treated as internal, logged, and replaced by a
// generic message so SQL, file paths and stack details never reach clients.
type UserError struct {
	msg string
	err error
}

func (e *UserError) Error() string { return e.msg }
func (e *UserError) Unwrap() error { return e.err }

// NewUserError formats a user-facing error. Like fmt.Errorf, a %w verb keeps
// the wrapped error available to errors.Is/As.
func NewUserError(format string, args ...any) error {
	err := fmt.Errorf(format, args...)
	return &UserError{msg: err.Error(), err: errors.Unwrap(err)}
}

// UserMessage returns the message of the first UserError in err's chain.
func UserMessage(err error) (string, bool) {
	var userErr *UserError
	if errors.As(err, &userErr) {
		return userErr.msg, true
	}
	return "", false
}
