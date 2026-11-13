/* START ALPHA CHANGE 1694 -- test independent snapshot command type parsing -- */
package build

import (
	"testing"

	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	"github.com/stretchr/testify/require"
)

/* START ALPHA CHANGE 1827 -- test canonical segmented snapshot command name -- */
func TestParseKind(t *testing.T) {
	t.Parallel()

	kind, err := parseKind("segmented")
	require.NoError(t, err)
	require.Equal(t, snapshotbuilder.KindSegmented, kind)

	kind, err = parseKind("checkpoint")
	require.NoError(t, err)
	require.Equal(t, snapshotbuilder.KindCheckpoint, kind)

	_, err = parseKind("recent")
	require.ErrorContains(t, err, "expected segmented or checkpoint")

	_, err = parseKind("unknown")
	require.ErrorContains(t, err, "expected segmented or checkpoint")
}

/* FINISH ALPHA CHANGE 1827 -- test canonical segmented snapshot command name -- */

/* FINISH ALPHA CHANGE 1694 -- test independent snapshot command type parsing -- */
