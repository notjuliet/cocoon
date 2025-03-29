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
	"github.com/bluesky-social/indigo/util"
)

func ResolveHandle(ctx context.Context, cli *http.Client, handle string) (string, error) {
	if cli == nil {
		cli = util.RobustHTTPClient()
	}

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

func FetchDidDoc(ctx context.Context, cli *http.Client, did string) (*DidDoc, error) {
	if cli == nil {
		cli = util.RobustHTTPClient()
	}

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

func FetchDidData(ctx context.Context, cli *http.Client, did string) (*DidData, error) {
	if cli == nil {
		cli = util.RobustHTTPClient()
	}

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

func FetchDidAuditLog(ctx context.Context, cli *http.Client, did string) (DidAuditLog, error) {
	if cli == nil {
		cli = util.RobustHTTPClient()
	}

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

func ResolveService(ctx context.Context, cli *http.Client, did string) (string, error) {
	if cli == nil {
		cli = util.RobustHTTPClient()
	}

	diddoc, err := FetchDidDoc(ctx, cli, did)
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
