package backtest

import "errors"

var (
	ErrNotFound  = errors.New("backtest run not found")
	ErrForbidden = errors.New("backtest forbidden")
)
