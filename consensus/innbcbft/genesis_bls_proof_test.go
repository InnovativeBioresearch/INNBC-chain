package innbcbft

import (
	"encoding/hex"
	"encoding/json"
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2040 -- reject unproved duplicate and mismatched genesis validator definitions -- */
func genesisPoPEntry(
	t *testing.T,
	scalar string,
	operator types.Address,
) *validator.GenesisValidator {
	t.Helper()

	key, err := bls.UnmarshalPrivateKey([]byte(scalar))
	require.NoError(t, err)
	proof, err := signer.MakeINNBCBLSPoP(key, operator)
	require.NoError(t, err)
	proofBytes, err := proof.Marshal()
	require.NoError(t, err)

	/* START ALPHA CHANGE 3424 -- retain proof regressions without obsolete VSET stake -- */
	return &validator.GenesisValidator{
		Address:  operator,
		BlsKey:   hex.EncodeToString(key.PublicKey().Marshal()),
		BlsProof: hex.EncodeToString(proofBytes),
	}
	/* FINISH ALPHA CHANGE 3424 -- retain proof regressions without obsolete VSET stake -- */
}

func TestGenesisBLSPoPValidation(t *testing.T) {
	first := genesisPoPEntry(t, "7", types.StringToAddress("0x1001"))
	second := genesisPoPEntry(t, "11", types.StringToAddress("0x1002"))

	config := &INNBCBFTConfig{
		InitialValidatorSet: []*validator.GenesisValidator{first, second},
		MinValidatorSetSize: 1,
		MaxValidatorSetSize: 10,
	}

	accounts, err := BuildGenesisValidatorSet(config)
	require.NoError(t, err)
	extra := BuildGenesisExtraData(accounts)
	require.NoError(t, ValidateGenesisExtraData(config, extra))

	raw, err := json.Marshal(first)
	require.NoError(t, err)
	var restored validator.GenesisValidator
	require.NoError(t, json.Unmarshal(raw, &restored))
	require.Equal(t, first.BlsProof, restored.BlsProof)
	_, err = restored.ToValidatorMetadata()
	require.NoError(t, err)

	missing := *first
	missing.BlsProof = ""
	wrongOperator := *first
	wrongOperator.Address = types.StringToAddress("0x1003")
	sameKeyDifferentOperator := genesisPoPEntry(
		t, "7", types.StringToAddress("0x1004"),
	)

	for name, entries := range map[string][]*validator.GenesisValidator{
		"empty":             nil,
		"nil entry":         {nil},
		"missing proof":     {&missing},
		"wrong operator":    {&wrongOperator},
		"duplicate address": {first, first},
		"duplicate key":     {first, sameKeyDifferentOperator},
	} {
		t.Run(name, func(t *testing.T) {
			bad := *config
			bad.InitialValidatorSet = entries
			_, err := BuildGenesisValidatorSet(&bad)
			require.Error(t, err)
		})
	}

	/* Different valid proofs must not authorize different header keys. */
	other := *config
	other.InitialValidatorSet = []*validator.GenesisValidator{
		genesisPoPEntry(t, "13", first.Address),
		second,
	}
	require.Error(t, ValidateGenesisExtraData(&other, extra))

	reordered := *config
	reordered.InitialValidatorSet = []*validator.GenesisValidator{second, first}
	require.Error(t, ValidateGenesisExtraData(&reordered, extra))

	tampered := append([]byte(nil), extra...)
	tampered[0] = 1
	require.Error(t, ValidateGenesisExtraData(config, tampered))
	require.Error(t, ValidateGenesisExtraData(config, nil))

	accounts[0].VotingPower = big.NewInt(2)
	require.Error(t, ValidateGenesisExtraData(config, BuildGenesisExtraData(accounts)))
}

/* FINISH ALPHA CHANGE 2040 -- reject unproved duplicate and mismatched genesis validator definitions -- */
