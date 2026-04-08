package feedback

import "errors"

var (
	ErrNotFound  = errors.New("feedback not found")
	ErrInvalid   = errors.New("invalid feedback input")
	ErrForbidden = errors.New("feedback forbidden")
)
