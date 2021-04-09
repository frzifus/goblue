package goblue

import "errors"

var (
	ErrNotImplemented       = errors.New("function not implemented")
	ErrNotAuthenticated     = errors.New("client not authenticated")
	ErrAuthenticationFailed = errors.New("client authentication failed")
	ErrUnknownBrand         = errors.New("unknown brand")
	ErrNoVehicleFound       = errors.New("no vehicle found")
)
