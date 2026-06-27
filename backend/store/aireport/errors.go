package aireport

import "errors"

var (
	ErrInvalidInput   = errors.New("ai report: invalid input")
	ErrReportNotFound = errors.New("ai report: not found")
)
