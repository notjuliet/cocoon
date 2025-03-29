package main

import (
	"fmt"
	"os"

	"github.com/haileyok/cocoon/server"
	_ "github.com/joho/godotenv/autoload"
	"github.com/urfave/cli/v2"
)

var Version = "dev"

func main() {
	app := &cli.App{
		Name:  "cocoon",
		Usage: "An atproto PDS",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "addr",
				Value:   ":8080",
				EnvVars: []string{"COCOON_ADDR"},
			},
			&cli.StringFlag{
				Name:    "db-name",
				Value:   "cocoon.db",
				EnvVars: []string{"COCOON_DB_NAME"},
			},
			&cli.StringFlag{
				Name:     "did",
				Required: true,
				EnvVars:  []string{"COCOON_DID"},
			},
			&cli.StringFlag{
				Name:     "hostname",
				Required: true,
				EnvVars:  []string{"COCOON_HOSTNAME"},
			},
			&cli.StringFlag{
				Name:     "rotation-key-path",
				Required: true,
				EnvVars:  []string{"COCOON_ROTATION_KEY_PATH"},
			},
			&cli.StringFlag{
				Name:     "jwk-path",
				Required: true,
				EnvVars:  []string{"COCOON_JWK_PATH"},
			},
			&cli.StringFlag{
				Name:     "contact-email",
				Required: true,
				EnvVars:  []string{"COCOON_CONTACT_EMAIL"},
			},
			&cli.StringSliceFlag{
				Name:     "relays",
				Required: true,
				EnvVars:  []string{"COCOON_RELAYS"},
			},
		},
		Commands: []*cli.Command{
			run,
		},
		ErrWriter: os.Stdout,
		Version:   Version,
	}

	app.Run(os.Args)
}

var run = &cli.Command{
	Name:  "run",
	Usage: "Start the cocoon PDS",
	Flags: []cli.Flag{},
	Action: func(cmd *cli.Context) error {
		s, err := server.New(&server.Args{
			Addr:            cmd.String("addr"),
			DbName:          cmd.String("db-name"),
			Did:             cmd.String("did"),
			Hostname:        cmd.String("hostname"),
			RotationKeyPath: cmd.String("rotation-key-path"),
			JwkPath:         cmd.String("jwk-path"),
			ContactEmail:    cmd.String("contact-email"),
			Version:         Version,
			Relays:          cmd.StringSlice("relays"),
		})
		if err != nil {
			fmt.Printf("error creating cocoon: %v", err)
			return err
		}

		if err := s.Serve(cmd.Context); err != nil {
			fmt.Printf("error starting cocoon: %v", err)
			return err
		}

		return nil
	},
}
