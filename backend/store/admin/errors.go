package admin

import "errors"

var (
	ErrInvalidInput           = errors.New("invalid admin input")
	ErrInvalidCredential      = errors.New("invalid admin credential")
	ErrAdminNotFound          = errors.New("admin not found")
	ErrForbidden              = errors.New("admin forbidden")
	ErrUnauthorized           = errors.New("admin unauthorized")
	ErrAIConfigInvalid        = errors.New("invalid ai provider config")
	ErrAIConfigCipherKeyUnset = errors.New("ai config cipher key is not configured")
)
