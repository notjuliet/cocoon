package server

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/indigo/events"
	"github.com/bluesky-social/indigo/util"
	"github.com/bluesky-social/indigo/xrpc"
	"github.com/go-playground/validator"
	"github.com/golang-jwt/jwt/v4"
	"github.com/haileyok/cocoon/identity"
	"github.com/haileyok/cocoon/internal/helpers"
	"github.com/haileyok/cocoon/models"
	"github.com/haileyok/cocoon/plc"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/lestrrat-go/jwx/v2/jwk"
	slogecho "github.com/samber/slog-echo"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Server struct {
	http       *http.Client
	httpd      *http.Server
	echo       *echo.Echo
	db         *gorm.DB
	plcClient  *plc.Client
	logger     *slog.Logger
	config     *config
	privateKey *ecdsa.PrivateKey
	repoman    *RepoMan
	evtman     *events.EventManager
	passport   *identity.Passport
}

type Args struct {
	Addr            string
	DbName          string
	Logger          *slog.Logger
	Version         string
	Did             string
	Hostname        string
	RotationKeyPath string
	JwkPath         string
	ContactEmail    string
	Relays          []string
}

type config struct {
	Version        string
	Did            string
	Hostname       string
	ContactEmail   string
	EnforcePeering bool
	Relays         []string
}

type CustomValidator struct {
	validator *validator.Validate
}

type ValidationError struct {
	error
	Field string
	Tag   string
}

func (cv *CustomValidator) Validate(i any) error {
	if err := cv.validator.Struct(i); err != nil {
		var validateErrors validator.ValidationErrors
		if errors.As(err, &validateErrors) && len(validateErrors) > 0 {
			first := validateErrors[0]
			return ValidationError{
				error: err,
				Field: first.Field(),
				Tag:   first.Tag(),
			}
		}

		return err
	}

	return nil
}

func (s *Server) handleSessionMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(e echo.Context) error {
		authheader := e.Request().Header.Get("authorization")
		if authheader == "" {
			return e.JSON(401, map[string]string{"error": "Unauthorized"})
		}

		pts := strings.Split(authheader, " ")
		if len(pts) != 2 {
			return helpers.ServerError(e, nil)
		}

		tokenstr := pts[1]

		token, err := new(jwt.Parser).Parse(tokenstr, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodECDSA); !ok {
				return nil, fmt.Errorf("unsupported signing method: %v", t.Header["alg"])
			}

			return s.privateKey.Public(), nil
		})
		if err != nil {
			s.logger.Error("error parsing jwt", "error", err)
			return helpers.InputError(e, to.StringPtr("InvalidToken"))
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok || !token.Valid {
			return helpers.InputError(e, to.StringPtr("InvalidToken"))
		}

		isRefresh := e.Request().URL.Path == "/xrpc/com.atproto.server.refreshSession"
		scope := claims["scope"].(string)

		if isRefresh && scope != "com.atproto.refresh" {
			return helpers.InputError(e, to.StringPtr("InvalidToken"))
		} else if !isRefresh && scope != "com.atproto.access" {
			return helpers.InputError(e, to.StringPtr("InvalidToken"))
		}

		table := "tokens"
		if isRefresh {
			table = "refresh_tokens"
		}

		type Result struct {
			Found bool
		}
		var result Result
		if err := s.db.Raw("SELECT EXISTS(SELECT 1 FROM "+table+" WHERE token = ?) AS found", tokenstr).Scan(&result).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return helpers.InputError(e, to.StringPtr("InvalidToken"))
			}

			s.logger.Error("error getting token from db", "error", err)
			return helpers.ServerError(e, nil)
		}

		if !result.Found {
			return helpers.InputError(e, to.StringPtr("InvalidToken"))
		}

		exp, ok := claims["exp"].(float64)
		if !ok {
			s.logger.Error("error getting iat from token")
			return helpers.ServerError(e, nil)
		}

		if exp < float64(time.Now().UTC().Unix()) {
			return helpers.InputError(e, to.StringPtr("ExpiredToken"))
		}

		e.Set("did", claims["sub"])

		repo, err := s.getRepoActorByDid(claims["sub"].(string))
		if err != nil {
			s.logger.Error("error fetching repo", "error", err)
			return helpers.ServerError(e, nil)
		}
		e.Set("repo", repo)

		e.Set("token", tokenstr)

		if err := next(e); err != nil {
			e.Error(err)
		}

		return nil
	}
}

func New(args *Args) (*Server, error) {
	if args.Addr == "" {
		return nil, fmt.Errorf("addr must be set")
	}

	if args.DbName == "" {
		return nil, fmt.Errorf("db name must be set")
	}

	if args.Did == "" {
		return nil, fmt.Errorf("cocoon did must be set")
	}

	if args.ContactEmail == "" {
		return nil, fmt.Errorf("cocoon contact email is required")
	}

	if _, err := syntax.ParseDID(args.Did); err != nil {
		return nil, fmt.Errorf("error parsing cocoon did: %w", err)
	}

	if args.Hostname == "" {
		return nil, fmt.Errorf("cocoon hostname must be set")
	}

	if args.Logger == nil {
		args.Logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{}))
	}

	e := echo.New()

	e.Pre(middleware.RemoveTrailingSlash())
	e.Pre(slogecho.New(args.Logger))
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOrigins:     []string{"*"},
		AllowHeaders:     []string{"*"},
		AllowMethods:     []string{"*"},
		AllowCredentials: true,
		MaxAge:           100_000_000,
	}))

	vdtor := validator.New()
	vdtor.RegisterValidation("atproto-handle", func(fl validator.FieldLevel) bool {
		if _, err := syntax.ParseHandle(fl.Field().String()); err != nil {
			return false
		}
		return true
	})
	vdtor.RegisterValidation("atproto-did", func(fl validator.FieldLevel) bool {
		if _, err := syntax.ParseDID(fl.Field().String()); err != nil {
			return false
		}
		return true
	})
	vdtor.RegisterValidation("atproto-rkey", func(fl validator.FieldLevel) bool {
		if _, err := syntax.ParseRecordKey(fl.Field().String()); err != nil {
			return false
		}
		return true
	})
	vdtor.RegisterValidation("atproto-nsid", func(fl validator.FieldLevel) bool {
		if _, err := syntax.ParseNSID(fl.Field().String()); err != nil {
			return false
		}
		return true
	})

	e.Validator = &CustomValidator{validator: vdtor}

	httpd := &http.Server{
		Addr:    args.Addr,
		Handler: e,
	}

	db, err := gorm.Open(sqlite.Open("cocoon.db"), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	rkbytes, err := os.ReadFile(args.RotationKeyPath)
	if err != nil {
		return nil, err
	}

	h := util.RobustHTTPClient()

	plcClient, err := plc.NewClient(&plc.ClientArgs{
		H:           h,
		Service:     "https://plc.directory",
		PdsHostname: args.Hostname,
		RotationKey: rkbytes,
	})
	if err != nil {
		return nil, err
	}

	jwkbytes, err := os.ReadFile(args.JwkPath)
	if err != nil {
		return nil, err
	}

	key, err := jwk.ParseKey(jwkbytes)
	if err != nil {
		return nil, err
	}

	var pkey ecdsa.PrivateKey
	if err := key.Raw(&pkey); err != nil {
		return nil, err
	}

	s := &Server{
		http:       h,
		httpd:      httpd,
		echo:       e,
		logger:     args.Logger,
		db:         db,
		plcClient:  plcClient,
		privateKey: &pkey,
		config: &config{
			Version:        args.Version,
			Did:            args.Did,
			Hostname:       args.Hostname,
			ContactEmail:   args.ContactEmail,
			EnforcePeering: false,
			Relays:         args.Relays,
		},
		evtman:   events.NewEventManager(events.NewMemPersister()),
		passport: identity.NewPassport(h, identity.NewMemCache(10_000)),
	}

	s.repoman = NewRepoMan(s) // TODO: this is way too lazy, stop it

	return s, nil
}

func (s *Server) addRoutes() {
	s.echo.GET("/", s.handleRoot)
	s.echo.GET("/xrpc/_health", s.handleHealth)
	s.echo.GET("/.well-known/did.json", s.handleWellKnown)
	s.echo.GET("/robots.txt", s.handleRobots)

	// public
	s.echo.GET("/xrpc/com.atproto.identity.resolveHandle", s.handleResolveHandle)
	s.echo.POST("/xrpc/com.atproto.server.createAccount", s.handleCreateAccount)
	s.echo.POST("/xrpc/com.atproto.server.createAccount", s.handleCreateAccount)
	s.echo.POST("/xrpc/com.atproto.server.createSession", s.handleCreateSession)
	s.echo.GET("/xrpc/com.atproto.server.describeServer", s.handleDescribeServer)

	s.echo.GET("/xrpc/com.atproto.repo.describeRepo", s.handleDescribeRepo)
	s.echo.GET("/xrpc/com.atproto.sync.listRepos", s.handleListRepos)
	s.echo.GET("/xrpc/com.atproto.repo.listRecords", s.handleListRecords)
	s.echo.GET("/xrpc/com.atproto.repo.getRecord", s.handleRepoGetRecord)
	s.echo.GET("/xrpc/com.atproto.sync.getRecord", s.handleSyncGetRecord)
	s.echo.GET("/xrpc/com.atproto.sync.getBlocks", s.handleGetBlocks)
	s.echo.GET("/xrpc/com.atproto.sync.getLatestCommit", s.handleSyncGetLatestCommit)
	s.echo.GET("/xrpc/com.atproto.sync.getRepoStatus", s.handleSyncGetRepoStatus)
	s.echo.GET("/xrpc/com.atproto.sync.getRepo", s.handleSyncGetRepo)
	s.echo.GET("/xrpc/com.atproto.sync.subscribeRepos", s.handleSyncSubscribeRepos)
	s.echo.GET("/xrpc/com.atproto.sync.listBlobs", s.handleSyncListBlobs)
	s.echo.GET("/xrpc/com.atproto.sync.getBlob", s.handleSyncGetBlob)

	// authed
	s.echo.GET("/xrpc/com.atproto.server.getSession", s.handleGetSession, s.handleSessionMiddleware)
	s.echo.POST("/xrpc/com.atproto.server.refreshSession", s.handleRefreshSession, s.handleSessionMiddleware)
	s.echo.POST("/xrpc/com.atproto.server.deleteSession", s.handleDeleteSession, s.handleSessionMiddleware)
	s.echo.POST("/xrpc/com.atproto.identity.updateHandle", s.handleIdentityUpdateHandle, s.handleSessionMiddleware)

	// repo
	s.echo.POST("/xrpc/com.atproto.repo.createRecord", s.handleCreateRecord, s.handleSessionMiddleware)
	s.echo.POST("/xrpc/com.atproto.repo.putRecord", s.handlePutRecord, s.handleSessionMiddleware)
	s.echo.POST("/xrpc/com.atproto.repo.applyWrites", s.handleApplyWrites, s.handleSessionMiddleware)
	s.echo.POST("/xrpc/com.atproto.repo.uploadBlob", s.handleRepoUploadBlob, s.handleSessionMiddleware)

	// stupid silly endpoints
	s.echo.GET("/xrpc/app.bsky.actor.getPreferences", s.handleActorGetPreferences, s.handleSessionMiddleware)
	s.echo.POST("/xrpc/app.bsky.actor.putPreferences", s.handleActorPutPreferences, s.handleSessionMiddleware)

	s.echo.GET("/xrpc/*", s.handleProxy, s.handleSessionMiddleware)
	s.echo.POST("/xrpc/*", s.handleProxy, s.handleSessionMiddleware)
}

func (s *Server) Serve(ctx context.Context) error {
	s.addRoutes()

	s.logger.Info("migrating...")

	s.db.AutoMigrate(
		&models.Actor{},
		&models.Repo{},
		&models.InviteCode{},
		&models.Token{},
		&models.RefreshToken{},
		&models.Block{},
		&models.Record{},
		&models.Blob{},
		&models.BlobPart{},
	)

	s.logger.Info("starting cocoon")

	go func() {
		if err := s.httpd.ListenAndServe(); err != nil {
			panic(err)
		}
	}()

	for _, relay := range s.config.Relays {
		cli := xrpc.Client{Host: relay}
		atproto.SyncRequestCrawl(context.TODO(), &cli, &atproto.SyncRequestCrawl_Input{
			Hostname: s.config.Hostname,
		})
	}

	<-ctx.Done()

	fmt.Println("shut down")

	return nil
}
