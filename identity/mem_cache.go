package identity

import (
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

type MemCache struct {
	docCache *expirable.LRU[string, *DidDoc]
	didCache *expirable.LRU[string, string]
}

func NewMemCache(size int) *MemCache {
	docCache := expirable.NewLRU[string, *DidDoc](size, nil, 5*time.Minute)
	didCache := expirable.NewLRU[string, string](size, nil, 5*time.Minute)

	return &MemCache{
		docCache: docCache,
		didCache: didCache,
	}
}

func (mc *MemCache) GetDoc(did string) (*DidDoc, bool) {
	return mc.docCache.Get(did)
}

func (mc *MemCache) PutDoc(did string, doc *DidDoc) error {
	mc.docCache.Add(did, doc)
	return nil
}

func (mc *MemCache) BustDoc(did string) error {
	mc.docCache.Remove(did)
	return nil
}

func (mc *MemCache) GetDid(handle string) (string, bool) {
	return mc.didCache.Get(handle)
}

func (mc *MemCache) PutDid(handle string, did string) error {
	mc.didCache.Add(handle, did)
	return nil
}

func (mc *MemCache) BustDid(handle string) error {
	mc.didCache.Remove(handle)
	return nil
}
