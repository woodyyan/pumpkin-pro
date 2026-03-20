package signal

import "errors"

var (
	ErrInvalidInput               = errors.New("invalid signal input")
	ErrNotFound                   = errors.New("signal resource not found")
	ErrConflict                   = errors.New("signal resource conflict")
	ErrForbidden                  = errors.New("signal forbidden")
	ErrWebhookMissing             = errors.New("webhook endpoint not configured")
	ErrWebhookOff                 = errors.New("webhook endpoint is disabled")
	ErrWebhookDeliveryUndelivered = errors.New("webhook delivery not delivered")
)
