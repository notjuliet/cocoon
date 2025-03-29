package server

import "github.com/labstack/echo/v4"

func (s *Server) handleHealth(e echo.Context) error {
	return e.JSON(200, map[string]string{
		"version": "cocoon " + s.config.Version,
	})
}
