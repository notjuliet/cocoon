package identity

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/bluesky-social/indigo/atproto/syntax"
)

func ResolveHandle(ctx context.Context, handle string) (string, error) {
	var did string

	_, err := syntax.ParseHandle(handle)
	if err != nil {
		return "", err
	}

	recs, err := net.LookupTXT(fmt.Sprintf("_atproto.%s", handle))
	if err == nil {
		for _, rec := range recs {
			if strings.HasPrefix(rec, "did=") {
				did = strings.Split(rec, "did=")[1]
				break
			}
		}
	} else {
		fmt.Printf("erorr getting txt records: %v\n", err)
	}

	if did == "" {
		req, err := http.NewRequestWithContext(
			ctx,
			"GET",
			fmt.Sprintf("https://%s/.well-known/atproto-did", handle),
			nil,
		)
		if err != nil {
			return "", nil
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			io.Copy(io.Discard, resp.Body)
			return "", fmt.Errorf("unable to resolve handle")
		}

		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", err
		}

		maybeDid := string(b)

		if _, err := syntax.ParseDID(maybeDid); err != nil {
			return "", fmt.Errorf("unable to resolve handle")
		}

		did = maybeDid
	}

	return did, nil
}

type DidDoc struct {
	Context             []string                   `json:"@context"`
	Id                  string                     `json:"id"`
	AlsoKnownAs         []string                   `json:"alsoKnownAs"`
	VerificationMethods []DidDocVerificationMethod `json:"verificationMethods"`
	Service             []DidDocService            `json:"service"`
}

type DidDocVerificationMethod struct {
	Id                 string `json:"id"`
	Type               string `json:"type"`
	Controller         string `json:"controller"`
	PublicKeyMultibase string `json:"publicKeyMultibase"`
}

type DidDocService struct {
	Id              string `json:"id"`
	Type            string `json:"type"`
	ServiceEndpoint string `json:"serviceEndpoint"`
}

type DidData struct {
	Did                 string                    `json:"did"`
	VerificationMethods map[string]string         `json:"verificationMethods"`
	RotationKeys        []string                  `json:"rotationKeys"`
	AlsoKnownAs         []string                  `json:"alsoKnownAs"`
	Services            map[string]DidDataService `json:"services"`
}

type DidDataService struct {
	Type     string `json:"type"`
	Endpoint string `json:"endpoint"`
}

type DidLog []DidLogEntry

type DidLogEntry struct {
	Sig                 string                    `json:"sig"`
	Prev                *string                   `json:"prev"`
	Type                string                    `json:"string"`
	Services            map[string]DidDataService `json:"services"`
	AlsoKnownAs         []string                  `json:"alsoKnownAs"`
	RotationKeys        []string                  `json:"rotationKeys"`
	VerificationMethods map[string]string         `json:"verificationMethods"`
}

type DidAuditEntry struct {
	Did       string      `json:"did"`
	Operation DidLogEntry `json:"operation"`
	Cid       string      `json:"cid"`
	Nullified bool        `json:"nullified"`
	CreatedAt string      `json:"createdAt"`
}

type DidAuditLog []DidAuditEntry

func FetchDidDoc(ctx context.Context, did string) (*DidDoc, error) {
	var ustr string
	if strings.HasPrefix(did, "did:plc:") {
		ustr = fmt.Sprintf("https://plc.directory/%s", did)
	} else if strings.HasPrefix(did, "did:web:") {
		ustr = fmt.Sprintf("https://%s/.well-known/did.json", strings.TrimPrefix(did, "did:web:"))
	} else {
		return nil, fmt.Errorf("did was not a supported did type")
	}

	req, err := http.NewRequestWithContext(ctx, "GET", ustr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("could not find identity in plc registry")
	}

	var diddoc DidDoc
	if err := json.NewDecoder(resp.Body).Decode(&diddoc); err != nil {
		return nil, err
	}

	return &diddoc, nil
}

func FetchDidData(ctx context.Context, did string) (*DidData, error) {
	var ustr string
	ustr = fmt.Sprintf("https://plc.directory/%s/data", did)

	req, err := http.NewRequestWithContext(ctx, "GET", ustr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("could not find identity in plc registry")
	}

	var diddata DidData
	if err := json.NewDecoder(resp.Body).Decode(&diddata); err != nil {
		return nil, err
	}

	return &diddata, nil
}

func FetchDidAuditLog(ctx context.Context, did string) (DidAuditLog, error) {
	var ustr string
	ustr = fmt.Sprintf("https://plc.directory/%s/log/audit", did)

	req, err := http.NewRequestWithContext(ctx, "GET", ustr, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("could not find identity in plc registry")
	}

	var didlog DidAuditLog
	if err := json.NewDecoder(resp.Body).Decode(&didlog); err != nil {
		return nil, err
	}

	return didlog, nil
}

func ResolveService(ctx context.Context, did string) (string, error) {
	diddoc, err := FetchDidDoc(ctx, did)
	if err != nil {
		return "", err
	}

	var service string
	for _, svc := range diddoc.Service {
		if svc.Id == "#atproto_pds" {
			service = svc.ServiceEndpoint
		}
	}

	if service == "" {
		return "", fmt.Errorf("could not find atproto_pds service in identity services")
	}

	return service, nil
}
