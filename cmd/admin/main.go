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
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/haileyok/cocoon/internal/helpers"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/urfave/cli/v2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func main() {
	app := cli.App{
		Name: "admin",
		Commands: cli.Commands{
			runCreateRotationKey,
			runCreatePrivateJwk,
			runCreateInviteCode,
			runResetPassword,
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

var runCreateInviteCode = &cli.Command{
	Name:  "create-invite-code",
	Usage: "creates an invite code",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "for",
			Usage: "optional did to assign the invite code to",
		},
		&cli.IntFlag{
			Name:  "uses",
			Usage: "number of times the invite code can be used",
			Value: 1,
		},
	},
	Action: func(cmd *cli.Context) error {
		db, err := newDb()
		if err != nil {
			return err
		}

		forDid := "did:plc:123"
		if cmd.String("for") != "" {
			did, err := syntax.ParseDID(cmd.String("for"))
			if err != nil {
				return err
			}

			forDid = did.String()
		}

		uses := cmd.Int("uses")

		code := fmt.Sprintf("%s-%s", helpers.RandomVarchar(8), helpers.RandomVarchar(8))

		if err := db.Exec("INSERT INTO invite_codes (did, code, remaining_use_count) VALUES (?, ?, ?)", forDid, code, uses).Error; err != nil {
			return err
		}

		fmt.Printf("New invite code created with %d uses: %s\n", uses, code)

		return nil
	},
}

var runResetPassword = &cli.Command{
	Name:  "reset-password",
	Usage: "resets a password",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "did",
			Usage: "did of the user who's password you want to reset",
		},
	},
	Action: func(cmd *cli.Context) error {
		db, err := newDb()
		if err != nil {
			return err
		}

		didStr := cmd.String("did")
		did, err := syntax.ParseDID(didStr)
		if err != nil {
			return err
		}

		newPass := fmt.Sprintf("%s-%s", helpers.RandomVarchar(12), helpers.RandomVarchar(12))
		hashed, err := bcrypt.GenerateFromPassword([]byte(newPass), 10)
		if err != nil {
			return err
		}

		if err := db.Exec("UPDATE repos SET password = ? WHERE did = ?", hashed, did.String()).Error; err != nil {
			return err
		}

		fmt.Printf("Password for %s has been reset to: %s", did.String(), newPass)

		return nil
	},
}

func newDb() (*gorm.DB, error) {
	return gorm.Open(sqlite.Open("cocoon.db"), &gorm.Config{})
}
