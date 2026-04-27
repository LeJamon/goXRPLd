package handlers

import (
	"encoding/base64"
	"encoding/json"

	"github.com/LeJamon/goXRPLd/codec/addresscodec"
	"github.com/LeJamon/goXRPLd/internal/rpc/types"
)

// ValidatorInfoMethod handles `validator_info`. Mirrors
// rippled/src/xrpld/rpc/handlers/ValidatorInfo.cpp; SDK tooling
// pattern-matches on the exact "not a validator" / rpcINVALID_PARAMS
// wire shape, so the error contract is load-bearing.
type ValidatorInfoMethod struct{ AdminHandler }

type validatorInfoResponse struct {
	MasterKey    string `json:"master_key,omitempty"`
	EphemeralKey string `json:"ephemeral_key,omitempty"`
	Manifest     string `json:"manifest,omitempty"`
	// Pointer preserves seq=0 (rippled emits `ret[seq] = *seq` regardless of value).
	Seq    *uint32 `json:"seq,omitempty"`
	Domain string  `json:"domain,omitempty"`
}

func (m *ValidatorInfoMethod) Handle(_ *types.RpcContext, _ json.RawMessage) (interface{}, *types.RpcError) {
	if types.Services == nil || len(types.Services.ValidatorPublicKey) == 0 {
		return nil, types.RpcErrorInvalidParams("not a validator")
	}

	validationPK := types.Services.ValidatorPublicKey
	if len(validationPK) != 33 {
		return nil, types.RpcErrorInternal("validator public key has invalid length")
	}

	var keyArr [33]byte
	copy(keyArr[:], validationPK)

	masterKey := keyArr
	if types.Services.Manifests != nil {
		masterKey = types.Services.Manifests.GetMasterKey(keyArr)
	}

	masterB58, err := addresscodec.EncodeNodePublicKey(masterKey[:])
	if err != nil {
		return nil, types.RpcErrorInternal("encode master key: " + err.Error())
	}
	resp := validatorInfoResponse{MasterKey: masterB58}

	// rippled: `if (mk == validationPK) return ret;`
	if masterKey != keyArr {
		ephB58, err := addresscodec.EncodeNodePublicKey(keyArr[:])
		if err != nil {
			return nil, types.RpcErrorInternal("encode ephemeral key: " + err.Error())
		}
		resp.EphemeralKey = ephB58

		if manifestBytes, ok := types.Services.Manifests.GetManifest(masterKey); ok {
			resp.Manifest = base64.StdEncoding.EncodeToString(manifestBytes)
		}
		if seq, ok := types.Services.Manifests.GetSequence(masterKey); ok {
			s := seq
			resp.Seq = &s
		}
		if domain, ok := types.Services.Manifests.GetDomain(masterKey); ok {
			resp.Domain = domain
		}
	}

	return resp, nil
}
