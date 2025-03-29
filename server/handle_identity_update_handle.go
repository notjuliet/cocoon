package server

import (
	"context"
	"strings"
	"time"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/events"
	"github.com/bluesky-social/indigo/util"
	"github.com/haileyok/cocoon/identity"
	"github.com/haileyok/cocoon/internal/helpers"
	"github.com/haileyok/cocoon/models"
	"github.com/labstack/echo/v4"
)

type ComAtprotoIdentityUpdateHandleRequest struct {
	Handle string `json:"handle" validate:"atproto-handle"`
}

func (s *Server) handleIdentityUpdateHandle(e echo.Context) error {
	repo := e.Get("repo").(*models.RepoActor)

	var req ComAtprotoIdentityUpdateHandleRequest
	if err := e.Bind(&req); err != nil {
		s.logger.Error("error binding", "error", err)
		return helpers.ServerError(e, nil)
	}

	req.Handle = strings.ToLower(req.Handle)

	if err := e.Validate(req); err != nil {
		return helpers.InputError(e, nil)
	}

	ctx := context.WithValue(e.Request().Context(), "skip-cache", true)

	if strings.HasPrefix(repo.Repo.Did, "did:plc:") {
		log, err := identity.FetchDidAuditLog(ctx, repo.Repo.Did)
		if err != nil {
			s.logger.Error("error fetching doc", "error", err)
			return helpers.ServerError(e, nil)
		}

		latest := log[len(log)-1]

		k, err := crypto.ParsePrivateBytesK256(repo.SigningKey)
		if err != nil {
			s.logger.Error("error parsing signing key", "error", err)
			return helpers.ServerError(e, nil)
		}

		op, err := s.plcClient.FormatAndSignAtprotoOp(k, req.Handle, latest.Operation.RotationKeys, &latest.Cid)
		if err != nil {
			return err
		}

		if err := s.plcClient.SendOperation(context.TODO(), repo.Repo.Did, op); err != nil {
			return err
		}
	}

	s.evtman.AddEvent(context.TODO(), &events.XRPCStreamEvent{
		RepoHandle: &atproto.SyncSubscribeRepos_Handle{
			Did:    repo.Repo.Did,
			Handle: req.Handle,
			Seq:    time.Now().UnixMicro(), // TODO: no
			Time:   time.Now().Format(util.ISO8601),
		},
	})

	s.evtman.AddEvent(context.TODO(), &events.XRPCStreamEvent{
		RepoIdentity: &atproto.SyncSubscribeRepos_Identity{
			Did:    repo.Repo.Did,
			Handle: to.StringPtr(req.Handle),
			Seq:    time.Now().UnixMicro(), // TODO: no
			Time:   time.Now().Format(util.ISO8601),
		},
	})

	if err := s.db.Exec("UPDATE actors SET handle = ? WHERE did = ?", req.Handle, repo.Repo.Did).Error; err != nil {
		s.logger.Error("error updating handle in db", "error", err)
		return helpers.ServerError(e, nil)
	}

	return nil
}
