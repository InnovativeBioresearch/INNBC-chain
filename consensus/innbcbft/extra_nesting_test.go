/* START ALPHA CHANGE 2980 -- verify embedded extra nesting rejection and supported validator metadata -- */
package innbcbft

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

func TestExtraNesting_RejectsBeforeRecursiveDecoding(t *testing.T) {
	nested := []byte{0xc0}
	for level := 1; level < 16; level++ {
		nested = append([]byte{0xc0 + byte(len(nested))}, nested...)
	}
	data := append(make([]byte, ExtraVanity), nested...)

	var extra Extra
	require.ErrorIs(t, extra.UnmarshalRLP(data), types.ErrRLPNestingTooDeep)

	// Header hashing uses this helper to decode and clean embedded extra.
	_, err := GetIbftExtraClean(data)
	require.ErrorIs(t, err, types.ErrRLPNestingTooDeep)

	require.NotPanics(t, func() {
		require.Error(t, extra.UnmarshalRLP(make([]byte, ExtraVanity-1)))
	})
	require.Error(t, extra.UnmarshalRLP(make([]byte, ExtraVanity)))

	valid := (&Extra{}).MarshalRLPTo(nil)
	valid = append(valid, 0x80)
	require.ErrorIs(t, extra.UnmarshalRLP(valid), types.ErrInvalidRLPStructure)
}

func TestExtraNesting_CurrentValidatorEncoding(t *testing.T) {
	identities := validator.NewTestValidatorsWithAliases(
		t,
		[]string{"A", "B"},
	).GetPublicIdentities()

	original := &Extra{
		Validators: &validator.ValidatorSetDelta{
			Added:   identities,
			Updated: identities[1:],
		},
		Parent:     &Signature{},
		Committed:  &Signature{},
		Checkpoint: &CheckpointData{EpochNumber: 1},
	}

	data := original.MarshalRLPTo(nil)
	require.NoError(t, types.ValidateRLPStructure(data[ExtraVanity:], 4))
	require.ErrorIs(
		t,
		types.ValidateRLPStructure(data[ExtraVanity:], 3),
		types.ErrRLPNestingTooDeep,
	)

	var decoded Extra
	require.NoError(t, decoded.UnmarshalRLP(data))
	require.True(t, original.Validators.Equals(decoded.Validators))
	require.Equal(t, original.Checkpoint, decoded.Checkpoint)

	_, err := GetIbftExtraClean(data)
	require.NoError(t, err)
}

/* FINISH ALPHA CHANGE 2980 -- verify embedded extra nesting rejection and supported validator metadata -- */
