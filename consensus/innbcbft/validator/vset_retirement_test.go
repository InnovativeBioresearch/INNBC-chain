/* START ALPHA CHANGE 3426 -- verify genesis identity serialization after VSET retirement -- */
package validator

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVSETRetirement_GenesisIdentityRoundTrip(t *testing.T) {
	original := NewTestValidators(t, 1).GetParamValidators()[0]
	original.MultiAddr = "/ip4/127.0.0.1/tcp/30301"

	encoded, err := json.Marshal(original)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(encoded, &fields))
	require.Len(t, fields, 4)
	require.Contains(t, fields, "address")
	require.Contains(t, fields, "blsKey")
	require.Contains(t, fields, "blsProof")
	require.Contains(t, fields, "multiAddr")
	require.NotContains(t, fields, "stake")
	require.NotContains(t, original.String(), "Stake=")

	var decoded GenesisValidator
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, *original, decoded)

	metadata, err := decoded.ToValidatorMetadata()
	require.NoError(t, err)
	require.Equal(t, original.Address, metadata.Address)
	require.Equal(t, "1", metadata.VotingPower.String())
	require.True(t, metadata.IsActive)
}

func TestVSETRetirement_RejectsExplicitGenesisStake(t *testing.T) {
	original := NewTestValidators(t, 1).GetParamValidators()[0]
	encoded, err := json.Marshal(original)
	require.NoError(t, err)

	for _, name := range []string{"stake", "Stake", "STAKE"} {
		for _, value := range []string{`"0x0"`, `"0x1"`, `0`, `null`, `{}`} {
			t.Run(name+"/"+value, func(t *testing.T) {
				var fields map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(encoded, &fields))
				fields[name] = json.RawMessage(value)

				payload, err := json.Marshal(fields)
				require.NoError(t, err)

				target := *original
				before := target

				err = json.Unmarshal(payload, &target)
				require.ErrorContains(t, err, "retired VSET accounting")
				require.ErrorContains(t, err, "regenerate genesis")
				require.Equal(t, before, target)
			})
		}
	}
}

/* FINISH ALPHA CHANGE 3426 -- verify genesis identity serialization after VSET retirement -- */
