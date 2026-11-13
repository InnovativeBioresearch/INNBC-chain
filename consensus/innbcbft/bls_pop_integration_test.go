package innbcbft

import (
	"math/big"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/contracts"
	"github.com/innovativebioresearch/innbc-chain/state"
	"github.com/innovativebioresearch/innbc-chain/state/runtime"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 2041 -- exercise rebuilt registry possession checks through the INNBC EVM -- */
type registryPoPFixture struct {
	transition *state.Transition
	reader     *ValidatorRegistryStateImpl
	operator   types.Address
	governance types.Address
}

func newRegistryPoPFixture(t *testing.T) *registryPoPFixture {
	t.Helper()

	operator := types.StringToAddress("0x10001")
	governance := types.StringToAddress("0x10002")
	token := types.StringToAddress("0x10003")

	/* Only staking transfers are stubbed. The registry, BLS contract,
	   hashing, pairing, storage and revert execution are real INNBC paths. */
	returnTrueCode := []byte{
		0x60, 0x01, 0x60, 0x00, 0x52, 0x60, 0x20, 0x60, 0x00, 0xf3,
	}

	transition := newTestTransition(t, map[types.Address]*chain.GenesisAccount{
		contracts.BLSContract: {
			Code: contractsapi.BLS.DeployedBytecode,
		},
		contracts.INNBCValidatorRegistryContract: {
			Code: contractsapi.INNBCValidatorRegistry.DeployedBytecode,
		},
		token: {
			Code: returnTrueCode,
		},
	})

	fixture := &registryPoPFixture{
		transition: transition,
		reader: NewValidatorRegistryState(
			contracts.INNBCValidatorRegistryContract,
			NewStateProvider(transition),
		),
		operator:   operator,
		governance: governance,
	}

	result := fixture.call(t, operator, "initialize",
		operator,
		big.NewInt(1),
		uint8(1),
		governance,
		token,
		big.NewInt(2),
	)
	require.NoError(t, result.Err)

	return fixture
}

func (f *registryPoPFixture) call(
	t *testing.T,
	caller types.Address,
	method string,
	args ...interface{},
) *runtime.ExecutionResult {
	t.Helper()

	abiMethod := contractsapi.INNBCValidatorRegistry.Abi.Methods[method]
	require.NotNil(t, abiMethod, "missing rebuilt registry method %s", method)

	input, err := abiMethod.Encode(args)
	require.NoError(t, err)

	return f.transition.Call2(
		caller,
		contracts.INNBCValidatorRegistryContract,
		input,
		big.NewInt(0),
		10_000_000,
	)
}

func registryProof(
	t *testing.T,
	scalar string,
	operator types.Address,
) ([]byte, []byte) {
	t.Helper()

	key, err := bls.UnmarshalPrivateKey([]byte(scalar))
	require.NoError(t, err)
	proof, err := signer.MakeINNBCBLSPoP(key, operator)
	require.NoError(t, err)
	proofBytes, err := proof.Marshal()
	require.NoError(t, err)

	return key.PublicKey().Marshal(), proofBytes
}

func TestRegistryBLSPoPAdmission(t *testing.T) {
	operator := types.StringToAddress("0x10001")
	key, proof := registryProof(t, "7", operator)
	otherKey, _ := registryProof(t, "11", operator)
	_, wrongOperatorProof := registryProof(t, "7", types.StringToAddress("0x20001"))

	altered := append([]byte(nil), proof...)
	altered[0] ^= 1

	for name, input := range map[string]struct {
		key   []byte
		proof []byte
	}{
		"wrong operator": {key, wrongOperatorProof},
		"wrong key":      {otherKey, proof},
		"altered proof":  {key, altered},
		"zero pair":      {make([]byte, 128), make([]byte, 64)},
		"zero key":       {make([]byte, 128), proof},
		"zero proof":     {key, make([]byte, 64)},
		"short key":      {key[:127], proof},
		"long key":       {append(append([]byte(nil), key...), 0), proof},
		"short proof":    {key, proof[:63]},
		"long proof":     {key, append(append([]byte(nil), proof...), 0)},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRegistryPoPFixture(t)
			result := fixture.call(t, operator, "registerValidator",
				operator, input.key, input.proof, "genesis-peer", big.NewInt(1),
			)
			require.Error(t, result.Err)

			record, err := fixture.reader.GetValidator(operator)
			require.NoError(t, err)
			require.False(t, record.Registered)
			require.False(t, record.BlsProofVerified)

			active, err := fixture.reader.GetActiveValidators()
			require.NoError(t, err)
			require.Empty(t, active)

			activated, err := fixture.reader.IsRegistryActivated()
			require.NoError(t, err)
			require.False(t, activated)
		})
	}

	fixture := newRegistryPoPFixture(t)
	result := fixture.call(t, operator, "registerValidator",
		operator, key, proof, "genesis-peer", big.NewInt(1),
	)
	require.NoError(t, result.Err)

	record, err := fixture.reader.GetValidator(operator)
	require.NoError(t, err)
	require.True(t, record.Registered)
	require.True(t, record.Whitelisted)
	require.True(t, record.BlsProofVerified)
	require.Equal(t, key, record.BlsKey)

	active, err := fixture.reader.GetActiveValidators()
	require.NoError(t, err)
	require.Equal(t, []types.Address{operator}, active)

	/* A later candidate proves possession but needs governance activation. */
	candidate := types.StringToAddress("0x20001")
	candidateKey, candidateProof := registryProof(t, "13", candidate)
	result = fixture.call(t, candidate, "registerValidator",
		candidate, candidateKey, candidateProof, "candidate-peer", big.NewInt(1),
	)
	require.NoError(t, result.Err)

	candidateRecord, err := fixture.reader.GetValidator(candidate)
	require.NoError(t, err)
	require.True(t, candidateRecord.BlsProofVerified)
	require.False(t, candidateRecord.Whitelisted)

	result = fixture.call(t, operator, "whitelistValidator", candidate)
	require.Error(t, result.Err)

	result = fixture.call(t, fixture.governance, "whitelistValidator", candidate)
	require.NoError(t, result.Err)

	active, err = fixture.reader.GetActiveValidators()
	require.NoError(t, err)
	require.ElementsMatch(t, []types.Address{operator, candidate}, active)

	/* Failed replacement preserves the old key and active status. */
	replacementKey, replacementProof := registryProof(t, "17", operator)
	before, err := fixture.reader.GetValidator(operator)
	require.NoError(t, err)

	result = fixture.call(t, operator, "updateBLSKey", replacementKey, proof)
	require.Error(t, result.Err)

	after, err := fixture.reader.GetValidator(operator)
	require.NoError(t, err)
	require.Equal(t, before, after)

	/* Unchanged updates are rejected before expensive verification. */
	result = fixture.call(t, operator, "updateBLSKey", key, proof)
	require.Error(t, result.Err)

	/* Successful replacement remains proved but loses activation. */
	result = fixture.call(t, operator, "updateBLSKey", replacementKey, replacementProof)
	require.NoError(t, result.Err)

	after, err = fixture.reader.GetValidator(operator)
	require.NoError(t, err)
	require.Equal(t, replacementKey, after.BlsKey)
	require.True(t, after.BlsProofVerified)
	require.False(t, after.Whitelisted)

	active, err = fixture.reader.GetActiveValidators()
	require.NoError(t, err)
	require.Equal(t, []types.Address{candidate}, active)

	result = fixture.call(t, fixture.governance, "whitelistValidator", operator)
	require.NoError(t, result.Err)

	active, err = fixture.reader.GetActiveValidators()
	require.NoError(t, err)
	require.ElementsMatch(t, []types.Address{operator, candidate}, active)
}

/* FINISH ALPHA CHANGE 2041 -- exercise rebuilt registry possession checks through the INNBC EVM -- */
