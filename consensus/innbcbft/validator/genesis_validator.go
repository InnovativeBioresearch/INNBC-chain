package validator

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"

	/* START ALPHA CHANGE 2002 -- import canonical genesis proof decoding and verification -- */
	"strings"
	/* FINISH ALPHA CHANGE 2002 -- import canonical genesis proof decoding and verification -- */

	"github.com/innovativebioresearch/innbc-chain/bls"
	/* START ALPHA CHANGE 2003 -- use the shared INNBC BLS possession verifier -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	/* FINISH ALPHA CHANGE 2003 -- use the shared INNBC BLS possession verifier -- */
	/* START ALPHA CHANGE 3420 -- remove obsolete genesis VSET amount parsing -- */
	"github.com/innovativebioresearch/innbc-chain/types"
	/* FINISH ALPHA CHANGE 3420 -- remove obsolete genesis VSET amount parsing -- */)

/* START ALPHA CHANGE 3420 -- retain genesis identities and reject retired VSET stake fields -- */
// GenesisValidator represents public information about genesis validator accounts.
type GenesisValidator struct {
	Address   types.Address
	BlsKey    string
	BlsProof  string
	MultiAddr string
}

type genesisValidatorRaw struct {
	Address   types.Address `json:"address"`
	BlsKey    string        `json:"blsKey"`
	BlsProof  string        `json:"blsProof"`
	MultiAddr string        `json:"multiAddr"`
}

func (v *GenesisValidator) MarshalJSON() ([]byte, error) {
	raw := &genesisValidatorRaw{
		Address:   v.Address,
		BlsKey:    v.BlsKey,
		BlsProof:  v.BlsProof,
		MultiAddr: v.MultiAddr,
	}

	return json.Marshal(raw)
}

func (v *GenesisValidator) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}

	for name := range fields {
		if strings.EqualFold(name, "stake") {
			return fmt.Errorf(
				"genesis validator field %q is retired VSET accounting; regenerate genesis without this field",
				name,
			)
		}
	}

	var raw genesisValidatorRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	v.Address = raw.Address
	v.BlsKey = raw.BlsKey
	v.BlsProof = raw.BlsProof
	v.MultiAddr = raw.MultiAddr

	return nil
}

/* FINISH ALPHA CHANGE 3420 -- retain genesis identities and reject retired VSET stake fields -- */

// UnmarshalBLSPublicKey unmarshals the hex encoded BLS public key
/* START ALPHA CHANGE 2007 -- reject noncanonical genesis BLS key lengths -- */
func (v *GenesisValidator) UnmarshalBLSPublicKey() (*bls.PublicKey, error) {
	if v == nil {
		return nil, fmt.Errorf("nil genesis validator")
	}

	decoded, err := hex.DecodeString(strings.TrimPrefix(v.BlsKey, "0x"))
	if err != nil {
		return nil, err
	}
	if len(decoded) != bls.PublicKeySize {
		return nil, fmt.Errorf(
			"invalid genesis BLS public key length: got %d, expected %d",
			len(decoded), bls.PublicKeySize,
		)
	}

	return bls.UnmarshalPublicKey(decoded)
}

/* FINISH ALPHA CHANGE 2007 -- reject noncanonical genesis BLS key lengths -- */

// ToValidatorMetadata creates ValidatorMetadata instance
/* START ALPHA CHANGE 2008 -- require possession before constructing genesis validator metadata -- */
func (v *GenesisValidator) ToValidatorMetadata() (*ValidatorMetadata, error) {
	if v == nil {
		return nil, fmt.Errorf("nil genesis validator")
	}

	publicKeyBytes, err := hex.DecodeString(strings.TrimPrefix(v.BlsKey, "0x"))
	if err != nil {
		return nil, fmt.Errorf("invalid genesis BLS key encoding: %w", err)
	}

	proofBytes, err := hex.DecodeString(strings.TrimPrefix(v.BlsProof, "0x"))
	if err != nil {
		return nil, fmt.Errorf("invalid genesis BLS proof encoding: %w", err)
	}

	blsKey, err := signer.VerifyINNBCBLSPoP(publicKeyBytes, v.Address, proofBytes)
	if err != nil {
		return nil, fmt.Errorf("invalid genesis BLS possession proof: %w", err)
	}

	return &ValidatorMetadata{
		Address:     v.Address,
		BlsKey:      blsKey,
		VotingPower: big.NewInt(1),
		IsActive:    true,
	}, nil
}

/* FINISH ALPHA CHANGE 2008 -- require possession before constructing genesis validator metadata -- */

/* START ALPHA CHANGE 3420 -- remove retired VSET amounts from genesis validator output -- */
// String implements fmt.Stringer interface.
func (v *GenesisValidator) String() string {
	return fmt.Sprintf("Address=%s; P2P Multi addr=%s; BLS Key=%s; BLS Proof=%s;",
		v.Address, v.MultiAddr, v.BlsKey, v.BlsProof)
}

/* FINISH ALPHA CHANGE 3420 -- remove retired VSET amounts from genesis validator output -- */
