package identity

import (
	"context"
	"sync"
)

type BackingCache interface {
	GetDoc(did string) (*DidDoc, bool)
	PutDoc(did string, doc *DidDoc) error
	BustDoc(did string) error

	GetDid(handle string) (string, bool)
	PutDid(handle string, did string) error
	BustDid(handle string) error
}

type Passport struct {
	bc BackingCache
	lk sync.Mutex
}

func NewPassport(bc BackingCache) *Passport {
	return &Passport{
		bc: bc,
		lk: sync.Mutex{},
	}
}

func (p *Passport) FetchDoc(ctx context.Context, did string) (*DidDoc, error) {
	skipCache, _ := ctx.Value("skip-cache").(bool)

	if !skipCache {
		cached, ok := p.bc.GetDoc(did)
		if ok {
			return cached, nil
		}
	}

	p.lk.Lock() // this is pretty pathetic, and i should rethink this. but for now, fuck it
	defer p.lk.Unlock()

	doc, err := FetchDidDoc(ctx, did)
	if err != nil {
		return nil, err
	}

	p.bc.PutDoc(did, doc)

	return doc, nil
}

func (p *Passport) ResolveHandle(ctx context.Context, handle string) (string, error) {
	skipCache, _ := ctx.Value("skip-cache").(bool)

	if !skipCache {
		cached, ok := p.bc.GetDid(handle)
		if ok {
			return cached, nil
		}
	}

	did, err := ResolveHandle(ctx, handle)
	if err != nil {
		return "", err
	}

	p.bc.PutDid(handle, did)

	return did, nil
}

func (p *Passport) BustDoc(ctx context.Context, did string) error {
	return p.bc.BustDoc(did)
}

func (p *Passport) BustDid(ctx context.Context, handle string) error {
	return p.bc.BustDid(handle)
}
