package auth

import "errors"

var (
	ErrInvalidInput       = errors.New("invalid auth input")
	ErrInvalidCredential  = errors.New("invalid credential")
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrUnauthorized       = errors.New("unauthorized")
	ErrForbidden          = errors.New("forbidden")
	ErrUserNotFound       = errors.New("user not found")
	ErrSessionNotFound    = errors.New("session not found")
)
