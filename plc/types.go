package plc

import (
	"encoding/json"

	"github.com/bluesky-social/indigo/atproto/data"
	cbg "github.com/whyrusleeping/cbor-gen"
)

type PlcOperation struct {
	Type                string                         `json:"type"`
	VerificationMethods map[string]string              `json:"verificationMethods"`
	RotationKeys        []string                       `json:"rotationKeys"`
	AlsoKnownAs         []string                       `json:"alsoKnownAs"`
	Services            map[string]PlcOperationService `json:"services"`
	Prev                *string                        `json:"prev"`
	Sig                 string                         `json:"sig,omitempty"`
}

type PlcOperationService struct {
	Type     string `json:"type"`
	Endpoint string `json:"endpoint"`
}

// This is kinda gross. We could just use cborgen i suppose?
func (po *PlcOperation) MarshalCBOR() ([]byte, error) {
	if po == nil {
		return cbg.CborNull, nil
	}

	b, err := json.Marshal(po)
	if err != nil {
		return nil, err
	}

	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}

	b, err = data.MarshalCBOR(m)
	if err != nil {
		return nil, err
	}

	return b, nil
}
