package server

import "github.com/labstack/echo/v4"

func (s *Server) handleRobots(e echo.Context) error {
	return e.String(200, "# Beep boop beep boop\n\n# Crawl me 🥺\nUser-agent: *\nAllow: /")
}
