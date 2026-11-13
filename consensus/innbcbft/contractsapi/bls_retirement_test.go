/* START ALPHA CHANGE 2920 -- verify regenerated artifacts omit retired BLS getters and retain active interfaces -- */
package contractsapi

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi/artifact"
	"github.com/stretchr/testify/require"
)

func TestBLSRetirement_EmbeddedSystemArtifacts(t *testing.T) {
	tests := []struct {
		name     string
		artifact *artifact.Artifact
	}{
		{
			name:     "System",
			artifact: System,
		},
		{
			name:     "INNBCNativeToken",
			artifact: INNBCNativeToken,
		},
		{
			name:     "ValidatorSet",
			artifact: ValidatorSet,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.artifact)
			require.NotNil(t, tc.artifact.Abi)
			require.NotEmpty(t, tc.artifact.Bytecode)
			require.NotEmpty(t, tc.artifact.DeployedBytecode)

			require.Nil(t, tc.artifact.Abi.GetMethod(
				"VALIDATOR_PKCHECK_PRECOMPILE",
			))
			require.Nil(t, tc.artifact.Abi.GetMethod(
				"VALIDATOR_PKCHECK_PRECOMPILE_GAS",
			))

			require.NotNil(t, tc.artifact.Abi.GetMethod(
				"NATIVE_TRANSFER_PRECOMPILE",
			))
			require.NotNil(t, tc.artifact.Abi.GetMethod(
				"NATIVE_TRANSFER_PRECOMPILE_GAS",
			))
		})
	}
}

func TestBLSRetirement_ActiveBLSArtifacts(t *testing.T) {
	require.NotNil(t, BLS)
	require.NotNil(t, BLS.Abi)
	require.NotEmpty(t, BLS.DeployedBytecode)
	require.NotNil(t, BLS.Abi.GetMethod("hashToPoint"))
	require.NotNil(t, BLS.Abi.GetMethod("verifySingle"))

	require.NotNil(t, INNBCValidatorRegistry)
	require.NotNil(t, INNBCValidatorRegistry.Abi)
	require.NotEmpty(t, INNBCValidatorRegistry.DeployedBytecode)
	require.NotNil(t, INNBCValidatorRegistry.Abi.GetMethod(
		"registerValidator",
	))
	require.NotNil(t, INNBCValidatorRegistry.Abi.GetMethod(
		"updateBLSKey",
	))
}

/* FINISH ALPHA CHANGE 2920 -- verify regenerated artifacts omit retired BLS getters and retain active interfaces -- */
