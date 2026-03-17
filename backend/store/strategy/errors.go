package strategy

import "errors"

var (
	ErrNotFound  = errors.New("strategy not found")
	ErrConflict  = errors.New("strategy already exists")
	ErrInvalid   = errors.New("invalid strategy payload")
	ErrForbidden = errors.New("strategy forbidden")
)
