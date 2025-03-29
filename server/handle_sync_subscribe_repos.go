package server

import (
	"fmt"
	"net/http"

	"github.com/bluesky-social/indigo/events"
	"github.com/bluesky-social/indigo/lex/util"
	"github.com/btcsuite/websocket"
	"github.com/labstack/echo/v4"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (s *Server) handleSyncSubscribeRepos(e echo.Context) error {
	conn, err := websocket.Upgrade(e.Response().Writer, e.Request(), e.Response().Header(), 1<<10, 1<<10)
	if err != nil {
		return err
	}

	s.logger.Info("new connection", "ua", e.Request().UserAgent())

	ctx := e.Request().Context()

	ident := e.RealIP() + "-" + e.Request().UserAgent()

	evts, cancel, err := s.evtman.Subscribe(ctx, ident, func(evt *events.XRPCStreamEvent) bool {
		return true
	}, nil)
	if err != nil {
		return err
	}
	defer cancel()

	header := events.EventHeader{Op: events.EvtKindMessage}
	for evt := range evts {
		wc, err := conn.NextWriter(websocket.BinaryMessage)
		if err != nil {
			return err
		}

		var obj util.CBOR

		switch {
		case evt.Error != nil:
			header.Op = events.EvtKindErrorFrame
			obj = evt.Error
		case evt.RepoCommit != nil:
			header.MsgType = "#commit"
			obj = evt.RepoCommit
		case evt.RepoHandle != nil:
			header.MsgType = "#handle"
			obj = evt.RepoHandle
		case evt.RepoIdentity != nil:
			header.MsgType = "#identity"
			obj = evt.RepoIdentity
		case evt.RepoAccount != nil:
			header.MsgType = "#account"
			obj = evt.RepoAccount
		case evt.RepoInfo != nil:
			header.MsgType = "#info"
			obj = evt.RepoInfo
		case evt.RepoMigrate != nil:
			header.MsgType = "#migrate"
			obj = evt.RepoMigrate
		case evt.RepoTombstone != nil:
			header.MsgType = "#tombstone"
			obj = evt.RepoTombstone
		default:
			return fmt.Errorf("unrecognized event kind")
		}

		if err := header.MarshalCBOR(wc); err != nil {
			return fmt.Errorf("failed to write header: %w", err)
		}

		if err := obj.MarshalCBOR(wc); err != nil {
			return fmt.Errorf("failed to write event: %w", err)
		}

		if err := wc.Close(); err != nil {
			return fmt.Errorf("failed to flush-close our event write: %w", err)
		}
	}

	return nil
}
