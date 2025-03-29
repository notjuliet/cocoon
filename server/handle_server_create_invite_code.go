package server

import (
	"github.com/google/uuid"
	"github.com/haileyok/cocoon/models"
	"github.com/labstack/echo/v4"
)

func (s *Server) handleCreateInviteCode(e echo.Context) error {
	ic := models.InviteCode{
		Code: uuid.NewString(),
	}

	return e.JSON(200, map[string]string{
		"code": ic.Code,
	})
}
