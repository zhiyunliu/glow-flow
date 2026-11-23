package errors

import "errors"

var (
	ErrInvalidNodeConfig  = errors.New("invalid node configuration")
	ErrInvalidChainConfig = errors.New("invalid chain configuration")
)
