package helpers

import (
	"math/rand"

	"github.com/labstack/echo/v4"
)

var letters = []rune("ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890")

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

func RandomVarchar(length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}
