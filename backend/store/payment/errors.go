package payment

import "errors"

var (
	ErrInvalidInput               = errors.New("payment: invalid input")
	ErrNotFound                   = errors.New("payment: not found")
	ErrConflict                   = errors.New("payment: conflict")
	ErrStripeDisabled             = errors.New("payment: stripe disabled")
	ErrStripeWebhookMisconfigured = errors.New("payment: stripe webhook misconfigured")
	ErrUnsupportedMode            = errors.New("payment: unsupported stripe mode")
)
