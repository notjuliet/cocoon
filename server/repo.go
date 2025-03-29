package server

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"

	"github.com/Azure/go-autorest/autorest/to"
	"github.com/bluesky-social/indigo/api/atproto"
	"github.com/bluesky-social/indigo/atproto/data"
	"github.com/bluesky-social/indigo/atproto/syntax"
	"github.com/bluesky-social/indigo/carstore"
	"github.com/bluesky-social/indigo/events"
	lexutil "github.com/bluesky-social/indigo/lex/util"
	"github.com/bluesky-social/indigo/repo"
	"github.com/bluesky-social/indigo/util"
	"github.com/haileyok/cocoon/blockstore"
	"github.com/haileyok/cocoon/models"
	blocks "github.com/ipfs/go-block-format"
	"github.com/ipfs/go-cid"
	cbor "github.com/ipfs/go-ipld-cbor"
	"github.com/ipld/go-car"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RepoMan struct {
	db    *gorm.DB
	s     *Server
	clock *syntax.TIDClock
}

func NewRepoMan(s *Server) *RepoMan {
	clock := syntax.NewTIDClock(0)

	return &RepoMan{
		s:     s,
		db:    s.db,
		clock: &clock,
	}
}

type OpType string

var (
	OpTypeCreate = OpType("com.atproto.repo.applyWrites#create")
	OpTypeUpdate = OpType("com.atproto.repo.applyWrites#update")
	OpTypeDelete = OpType("com.atproto.repo.applyWrites#delete")
)

func (ot OpType) String() string {
	return ot.String()
}

type Op struct {
	Type       OpType          `json:"$type"`
	Collection string          `json:"collection"`
	Rkey       *string         `json:"rkey,omitempty"`
	Validate   *bool           `json:"validate,omitempty"`
	SwapRecord *string         `json:"swapRecord,omitempty"`
	Record     *MarshalableMap `json:"record,omitempty"`
}

type MarshalableMap map[string]any

type FirehoseOp struct {
	Cid    cid.Cid
	Path   string
	Action string
}

func (mm *MarshalableMap) MarshalCBOR(w io.Writer) error {
	data, err := data.MarshalCBOR(*mm)
	if err != nil {
		return err
	}

	w.Write(data)

	return nil
}

// TODO make use of swap commit
func (rm *RepoMan) applyWrites(urepo models.Repo, writes []Op, swapCommit *string) error {
	rootcid, err := cid.Cast(urepo.Root)
	if err != nil {
		return err
	}

	dbs := blockstore.New(urepo.Did, rm.db)
	r, err := repo.OpenRepo(context.TODO(), dbs, rootcid)

	entries := []models.Record{}

	for i, op := range writes {
		if op.Type != OpTypeCreate && op.Rkey == nil {
			return fmt.Errorf("invalid rkey")
		} else if op.Rkey == nil {
			op.Rkey = to.StringPtr(rm.clock.Next().String())
			writes[i].Rkey = op.Rkey
		}

		_, err := syntax.ParseRecordKey(*op.Rkey)
		if err != nil {
			return err
		}

		switch op.Type {
		case OpTypeCreate:
			nc, err := r.PutRecord(context.TODO(), op.Collection+"/"+*op.Rkey, op.Record)
			if err != nil {
				return err
			}

			d, _ := data.MarshalCBOR(*op.Record)
			entries = append(entries, models.Record{
				Did:       urepo.Did,
				CreatedAt: rm.clock.Next().String(),
				Nsid:      op.Collection,
				Rkey:      *op.Rkey,
				Cid:       nc.String(),
				Value:     d,
			})
		case OpTypeDelete:
			err := r.DeleteRecord(context.TODO(), op.Collection+"/"+*op.Rkey)
			if err != nil {
				return err
			}
		case OpTypeUpdate:
			nc, err := r.UpdateRecord(context.TODO(), op.Collection+"/"+*op.Rkey, op.Record)
			if err != nil {
				return err
			}

			d, _ := data.MarshalCBOR(*op.Record)
			entries = append(entries, models.Record{
				Did:       urepo.Did,
				CreatedAt: rm.clock.Next().String(),
				Nsid:      op.Collection,
				Rkey:      *op.Rkey,
				Cid:       nc.String(),
				Value:     d,
			})
		}
	}

	newroot, rev, err := r.Commit(context.TODO(), urepo.SignFor)
	if err != nil {
		return err
	}

	buf := new(bytes.Buffer)

	hb, err := cbor.DumpObject(&car.CarHeader{
		Roots:   []cid.Cid{newroot},
		Version: 1,
	})

	if _, err := carstore.LdWrite(buf, hb); err != nil {
		return err
	}

	diffops, err := r.DiffSince(context.TODO(), rootcid)
	if err != nil {
		return err
	}

	ops := make([]*atproto.SyncSubscribeRepos_RepoOp, 0, len(diffops))

	for _, op := range diffops {
		switch op.Op {
		case "add", "mut":
			kind := "create"
			if op.Op == "mut" {
				kind = "update"
			}

			ll := lexutil.LexLink(op.NewCid)
			ops = append(ops, &atproto.SyncSubscribeRepos_RepoOp{
				Action: kind,
				Path:   op.Rpath,
				Cid:    &ll,
			})

		case "del":
			ops = append(ops, &atproto.SyncSubscribeRepos_RepoOp{
				Action: "delete",
				Path:   op.Rpath,
				Cid:    nil,
			})
		}

		blk, err := dbs.Get(context.TODO(), op.NewCid)
		if err != nil {
			return err
		}

		if _, err := carstore.LdWrite(buf, blk.Cid().Bytes(), blk.RawData()); err != nil {
			return err
		}
	}

	for _, op := range dbs.GetLog() {
		if _, err := carstore.LdWrite(buf, op.Cid().Bytes(), op.RawData()); err != nil {
			return err
		}
	}

	var blobs []lexutil.LexLink
	for _, entry := range entries {
		if err := rm.s.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "did"}, {Name: "nsid"}, {Name: "rkey"}},
			UpdateAll: true,
		}).Create(&entry).Error; err != nil {
			return err
		}

		// we should actually check the type (i.e. delete, create,., update) here but we'll do it later
		cids, err := rm.incrementBlobRefs(urepo, entry.Value)
		if err != nil {
			return err
		}

		for _, c := range cids {
			blobs = append(blobs, lexutil.LexLink(c))
		}
	}

	rm.s.evtman.AddEvent(context.TODO(), &events.XRPCStreamEvent{
		RepoCommit: &atproto.SyncSubscribeRepos_Commit{
			Repo:   urepo.Did,
			Blocks: buf.Bytes(),
			Blobs:  blobs,
			Rev:    rev,
			Since:  &urepo.Rev,
			Commit: lexutil.LexLink(newroot),
			Time:   time.Now().Format(util.ISO8601),
			Ops:    ops,
			TooBig: false,
		},
	})

	if err := dbs.UpdateRepo(context.TODO(), newroot, rev); err != nil {
		return err
	}

	return nil
}

func (rm *RepoMan) getRecordProof(urepo models.Repo, collection, rkey string) (cid.Cid, []blocks.Block, error) {
	c, err := cid.Cast(urepo.Root)
	if err != nil {
		return cid.Undef, nil, err
	}

	dbs := blockstore.New(urepo.Did, rm.db)
	bs := util.NewLoggingBstore(dbs)

	r, err := repo.OpenRepo(context.TODO(), bs, c)
	if err != nil {
		return cid.Undef, nil, err
	}

	_, _, err = r.GetRecordBytes(context.TODO(), collection+"/"+rkey)
	if err != nil {
		return cid.Undef, nil, err
	}

	return c, bs.GetLoggedBlocks(), nil
}

func (rm *RepoMan) incrementBlobRefs(urepo models.Repo, cbor []byte) ([]cid.Cid, error) {
	cids, err := getBlobCidsFromCbor(cbor)
	if err != nil {
		return nil, err
	}

	for _, c := range cids {
		if err := rm.db.Exec("UPDATE blobs SET ref_count = ref_count + 1 WHERE did = ? AND cid = ?", urepo.Did, c.Bytes()).Error; err != nil {
			return nil, err
		}
	}

	return cids, nil
}

// to be honest, we could just store both the cbor and non-cbor in []entries above to avoid an additional
// unmarshal here. this will work for now though
func getBlobCidsFromCbor(cbor []byte) ([]cid.Cid, error) {
	var cids []cid.Cid

	decoded, err := data.UnmarshalCBOR(cbor)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling cbor: %w", err)
	}

	var deepiter func(interface{}) error
	deepiter = func(item interface{}) error {
		switch val := item.(type) {
		case map[string]interface{}:
			if val["$type"] == "blob" {
				if ref, ok := val["ref"].(string); ok {
					c, err := cid.Parse(ref)
					if err != nil {
						return err
					}
					cids = append(cids, c)
				}
				for _, v := range val {
					return deepiter(v)
				}
			}
		case []interface{}:
			for _, v := range val {
				deepiter(v)
			}
		}

		return nil
	}

	if err := deepiter(decoded); err != nil {
		return nil, err
	}

	return cids, nil
}
