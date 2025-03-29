package server

import (
	"github.com/labstack/echo/v4"
)

func (s *Server) handleWellKnown(e echo.Context) error {
	return e.JSON(200, map[string]any{
		"@context": []string{
			"https://www.w3.org/ns/did/v1",
		},
		"id": s.config.Did,
		"service": []map[string]string{
			{
				"id":              "#atproto_pds",
				"type":            "AtprotoPersonalDataServer",
				"serviceEndpoint": "https://" + s.config.Hostname,
			},
		},
	})
}
