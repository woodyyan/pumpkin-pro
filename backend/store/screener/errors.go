package screener

import "errors"

var (
	ErrNotFound  = errors.New("watchlist not found")
	ErrForbidden = errors.New("watchlist forbidden")
	ErrConflict  = errors.New("watchlist already exists")
	ErrInvalid   = errors.New("invalid watchlist input")
	ErrLimit     = errors.New("watchlist limit reached")
)
