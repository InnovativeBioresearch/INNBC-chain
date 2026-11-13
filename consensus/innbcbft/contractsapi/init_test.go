package contractsapi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArtifactNotEmpty(t *testing.T) {
	/* START ALPHA CHANGE 467 -- remove legacy bridge artifact assertions -- */
	/* FINISH ALPHA CHANGE 467 -- remove legacy bridge artifact assertions -- */

	/* START ALPHA CHANGE 3763 -- retain active BLS artifact checks after standalone G2 retirement -- */
	require.NotEmpty(t, BLS.Bytecode)
	require.NotEmpty(t, BLS.DeployedBytecode)
	require.NotEmpty(t, BLS.Abi)
	/* FINISH ALPHA CHANGE 3763 -- retain active BLS artifact checks after standalone G2 retirement -- */
}
