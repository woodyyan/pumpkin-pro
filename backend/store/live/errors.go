package live

import "errors"

var (
	ErrNotFound           = errors.New("watchlist symbol not found")
	ErrConflict           = errors.New("watchlist symbol already exists")
	ErrInvalidSymbol      = errors.New("invalid symbol")
	ErrDataSourceDown     = errors.New("data source unavailable")
	ErrWarmupNotReady     = errors.New("warmup not ready")
	ErrActiveSymbolNeeded = errors.New("active symbol is required")
)
