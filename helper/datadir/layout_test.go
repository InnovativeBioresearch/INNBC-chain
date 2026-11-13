/* START ALPHA CHANGE 1611 -- test canonical chain data and identity paths -- */
package datadir

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNodeDataLayout(t *testing.T) {
	root := filepath.Join("var", "lib", "innbc", "node1")

	require.Equal(
		t,
		filepath.Join(root, "chain-data", "blockchain"),
		Blockchain(root),
	)
	require.Equal(
		t,
		filepath.Join(root, "chain-data", "trie"),
		Trie(root),
	)
	require.Equal(
		t,
		filepath.Join(root, "chain-data", "consensus"),
		Consensus(root),
	)
	require.Equal(
		t,
		filepath.Join(root, "keys", "validator-ecdsa", "validator.key"),
		ValidatorKey(root),
	)
	require.Equal(
		t,
		filepath.Join(root, "keys", "validator-bls", "validator-bls.key"),
		ValidatorBLSKey(root),
	)
	require.Equal(
		t,
		filepath.Join(root, "keys", "libp2p", "libp2p.key"),
		LibP2PKey(root),
	)
}

/* FINISH ALPHA CHANGE 1611 -- test canonical chain data and identity paths -- */
