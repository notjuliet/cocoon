package plc

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/bluesky-social/indigo/atproto/crypto"
	"github.com/bluesky-social/indigo/atproto/data"
	"github.com/bluesky-social/indigo/util"
)

type Client struct {
	h *http.Client

	service     string
	rotationKey *crypto.PrivateKeyK256
	recoveryKey string
	pdsHostname string
}

type ClientArgs struct {
	Service     string
	RotationKey []byte
	RecoveryKey string
	PdsHostname string
}

func NewClient(args *ClientArgs) (*Client, error) {
	if args.Service == "" {
		args.Service = "https://plc.directory"
	}

	rk, err := crypto.ParsePrivateBytesK256([]byte(args.RotationKey))
	if err != nil {
		return nil, err
	}

	return &Client{
		h:           util.RobustHTTPClient(),
		service:     args.Service,
		rotationKey: rk,
		recoveryKey: args.RecoveryKey,
		pdsHostname: args.PdsHostname,
	}, nil
}

func (c *Client) CreateDID(ctx context.Context, sigkey *crypto.PrivateKeyK256, recovery string, handle string) (string, map[string]any, error) {
	pubrotkey, err := c.rotationKey.PublicKey()
	if err != nil {
		return "", nil, err
	}

	// todo
	rotationKeys := []string{pubrotkey.DIDKey()}
	if c.recoveryKey != "" {
		rotationKeys = []string{c.recoveryKey, rotationKeys[0]}
	}
	if recovery != "" {
		rotationKeys = func(recovery string) []string {
			newRotationKeys := []string{recovery}
			for _, k := range rotationKeys {
				newRotationKeys = append(newRotationKeys, k)
			}
			return newRotationKeys
		}(recovery)
	}

	op, err := c.FormatAndSignAtprotoOp(sigkey, handle, rotationKeys, nil)
	if err != nil {
		return "", nil, err
	}

	did, err := didForCreateOp(op)
	if err != nil {
		return "", nil, err
	}

	return did, op, nil
}

func (c *Client) UpdateUserHandle(ctx context.Context, didstr string, nhandle string) error {
	return nil
}

func (c *Client) FormatAndSignAtprotoOp(sigkey *crypto.PrivateKeyK256, handle string, rotationKeys []string, prev *string) (map[string]any, error) {
	pubsigkey, err := sigkey.PublicKey()
	if err != nil {
		return nil, err
	}

	op := map[string]any{
		"type": "plc_operation",
		"verificationMethods": map[string]string{
			"atproto": pubsigkey.DIDKey(),
		},
		"rotationKeys": rotationKeys,
		"alsoKnownAs":  []string{"at://" + handle},
		"services": map[string]any{
			"atproto_pds": map[string]string{
				"type":     "AtprotoPersonalDataServer",
				"endpoint": "https://" + c.pdsHostname,
			},
		},
		"prev": prev,
	}

	b, err := data.MarshalCBOR(op)
	if err != nil {
		return nil, err
	}

	sig, err := c.rotationKey.HashAndSign(b)
	if err != nil {
		return nil, err
	}

	op["sig"] = base64.RawURLEncoding.EncodeToString(sig)

	return op, nil
}

func didForCreateOp(op map[string]any) (string, error) {
	b, err := data.MarshalCBOR(op)
	if err != nil {
		return "", err
	}

	h := sha256.New()
	h.Write(b)
	bs := h.Sum(nil)

	b32 := strings.ToLower(base32.StdEncoding.EncodeToString(bs))

	return "did:plc:" + b32[0:24], nil
}

func (c *Client) SendOperation(ctx context.Context, did string, op any) error {
	b, err := json.Marshal(op)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.service+"/"+url.QueryEscape(did), bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	req.Header.Add("content-type", "application/json")

	resp, err := c.h.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Println(resp.StatusCode)

	b, err = io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	fmt.Println(string(b))

	return nil
}
