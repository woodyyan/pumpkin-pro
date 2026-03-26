package live

import "errors"

var (
	ErrNotFound           = errors.New("watchlist symbol not found")
	ErrConflict           = errors.New("watchlist symbol already exists")
	ErrInvalidSymbol      = errors.New("invalid symbol")
	ErrSymbolNotExist     = errors.New("该股票代码不存在或暂无行情数据，请检查后重试")
	ErrInvalidArgument    = errors.New("invalid argument")
	ErrDataSourceDown     = errors.New("data source unavailable")
	ErrWarmupNotReady     = errors.New("warmup not ready")
	ErrActiveSymbolNeeded = errors.New("active symbol is required")
)
