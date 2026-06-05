package auth

import "errors"

var (
	ErrInvalidInput       = errors.New("invalid auth input")
	ErrEmailRequired      = errors.New("email required")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrPasswordRequired   = errors.New("password required")
	ErrPasswordTooShort   = errors.New("password too short")
	ErrInvalidCredential  = errors.New("invalid credential")
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrRateLimited        = errors.New("rate limited")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrUserNotFound       = errors.New("user not found")
	ErrSessionNotFound    = errors.New("session not found")
	ErrResetTokenNotFound = errors.New("password reset token not found")
	ErrResetTokenExpired  = errors.New("password reset token expired")
	ErrResetTokenConsumed = errors.New("password reset token consumed")
)

type RateLimitError struct {
	RetryAfterSeconds int
}

func (e *RateLimitError) Error() string {
	return ErrRateLimited.Error()
}

func (e *RateLimitError) Unwrap() error {
	return ErrRateLimited
}

func (e *RateLimitError) RetryAfter() int {
	if e == nil || e.RetryAfterSeconds <= 0 {
		return 1
	}
	return e.RetryAfterSeconds
}
