package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/urfave/cli/v2"
)

func main() {
	app := cli.App{
		Name: "admin",
		Commands: cli.Commands{
			runCreateRotationKey,
			runCreatePrivateJwk,
		},
		ErrWriter: os.Stdout,
	}

	app.Run(os.Args)
}

var runCreateRotationKey = &cli.Command{
	Name:  "create-rotation-key",
	Usage: "creates a rotation key for your pds",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "out",
			Required: true,
			Usage:    "output file for your rotation key",
		},
	},
	Action: func(cmd *cli.Context) error {
		key, err := crypto.GeneratePrivateKeyK256()
		if err != nil {
			return err
		}

		bytes := key.Bytes()

		if err := os.WriteFile(cmd.String("out"), bytes, 0644); err != nil {
			return err
		}

		return nil
	},
}

var runCreatePrivateJwk = &cli.Command{
	Name:  "create-private-jwk",
	Usage: "creates a private jwk for your pds",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:     "out",
			Required: true,
			Usage:    "output file for your jwk",
		},
	},
	Action: func(cmd *cli.Context) error {
		privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}

		key, err := jwk.FromRaw(privKey)
		if err != nil {
			return err
		}

		kid := fmt.Sprintf("%d", time.Now().Unix())

		if err := key.Set(jwk.KeyIDKey, kid); err != nil {
			return err
		}

		b, err := json.Marshal(key)
		if err != nil {
			return err
		}

		if err := os.WriteFile(cmd.String("out"), b, 0644); err != nil {
			return err
		}

		return nil
	},
}
