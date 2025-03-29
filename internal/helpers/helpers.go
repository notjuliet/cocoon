package helpers

import "github.com/labstack/echo/v4"

func InputError(e echo.Context, custom *string) error {
	msg := "InvalidRequest"
	if custom != nil {
		msg = *custom
	}
	return genericError(e, 400, msg)
}

func ServerError(e echo.Context, suffix *string) error {
	msg := "Internal server error"
	if suffix != nil {
		msg += ". " + *suffix
	}
	return genericError(e, 400, msg)
}

func genericError(e echo.Context, code int, msg string) error {
	return e.JSON(code, map[string]string{
		"error": msg,
	})
}
