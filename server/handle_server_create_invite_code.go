package server

import (
	"github.com/google/uuid"
	"github.com/haileyok/cocoon/internal/helpers"
	"github.com/haileyok/cocoon/models"
	"github.com/labstack/echo/v4"
)

type ComAtprotoServerCreateInviteCodeRequest struct {
	UseCount   int    `json:"useCount" validate:"required"`
	ForAccount string `json:"forAccount" validate:"atproto-did"`
}

type ComAtprotoServerCreateInviteCodeResponse struct {
	Code string `json:"code"`
}

func (s *Server) handleCreateInviteCode(e echo.Context) error {
	var req ComAtprotoServerCreateInviteCodeRequest
	if err := e.Bind(&req); err != nil {
		s.logger.Error("error binding", "error", err)
		return helpers.ServerError(e, nil)
	}

	ic := uuid.NewString()

	if err := s.db.Create(&models.InviteCode{
		Code:              ic,
		Did:               req.ForAccount,
		RemainingUseCount: req.UseCount,
	}).Error; err != nil {
		s.logger.Error("error creating invite code", "error", err)
		return helpers.ServerError(e, nil)
	}

	return e.JSON(200, ComAtprotoServerCreateInviteCodeResponse{
		Code: ic,
	})
}
