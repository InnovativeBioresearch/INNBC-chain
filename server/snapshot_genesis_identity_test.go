/* START ALPHA CHANGE 3214 -- prevent cross-genesis snapshot replacement while preserving fresh bootstrap policy -- */
package server

import (
	"bytes"
	"fmt"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/snapshotbuilder"
	statepkg "github.com/innovativebioresearch/innbc-chain/state"
	itrie "github.com/innovativebioresearch/innbc-chain/state/immutable-trie"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/stretchr/testify/require"
)

// Build a second real genesis trie with identical non-state header fields.
// Changing one allocation makes its genesis identity differ from the candidate.
func newSnapshotGenesisIdentityLocalFixture(
	t *testing.T,
	candidate *snapshotStartupFixture,
) *snapshotStartupFixture {
	t.Helper()

	localChain := *candidate.chain
	localGenesis := *candidate.chain.Genesis
	localChain.Genesis = &localGenesis

	localGenesis.Alloc = make(
		map[types.Address]*chain.GenesisAccount,
		len(candidate.chain.Genesis.Alloc),
	)
	for address, account := range candidate.chain.Genesis.Alloc {
		localGenesis.Alloc[address] = account
	}

	address := types.StringToAddress("0x1001")
	original := localGenesis.Alloc[address]
	require.NotNil(t, original)
	require.NotNil(t, original.Balance)

	account := *original
	account.Balance = new(big.Int).Add(original.Balance, big.NewInt(1))
	localGenesis.Alloc[address] = &account

	local := *candidate
	local.chain = &localChain
	local.sourceDataDir = filepath.Join(t.TempDir(), "local-source")

	trieStorage, err := itrie.NewMDBXStorageWithMapSize(
		datadir.Trie(local.sourceDataDir),
		hclog.NewNullLogger(),
		snapshotIntegrationMDBXMapSize,
	)
	require.NoError(t, err)

	executor := statepkg.NewExecutor(
		localChain.Params,
		itrie.NewState(trieStorage),
		hclog.NewNullLogger(),
	)
	root, writeErr := executor.WriteGenesis(
		localGenesis.Alloc,
		types.ZeroHash,
	)
	closeErr := trieStorage.Close()
	require.NoError(t, writeErr)
	require.NoError(t, closeErr)

	require.NotEqual(t, candidate.genesisRoot, root)
	local.genesisRoot = root

	return &local
}

func TestSnapshotGenesisIdentity_LocalChainAnchor(t *testing.T) {
	for _, mode := range []string{"recent", "full"} {
		t.Run(mode, func(t *testing.T) {
			candidate := newSnapshotStartupFixture(t, mode)
			local := newSnapshotGenesisIdentityLocalFixture(t, candidate)

			latest, err := snapshotbuilder.LoadLatest(
				filepath.Join(candidate.snapshotRoot, "latest.json"),
			)
			require.NoError(t, err)

			manifest, err := snapshotbuilder.ValidatePublishedArtifact(
				filepath.Join(candidate.snapshotRoot, latest.SnapshotDir),
				latest.SnapshotKind,
				1,
				hclog.NewNullLogger(),
			)
			require.NoError(t, err)
			require.Equal(t, uint64(5), manifest.BlockNumber)

			candidateGenesis := candidate.chain.Genesis.GenesisHeader()
			candidateGenesis.StateRoot = candidate.genesisRoot
			candidateGenesis.ComputeHash()
			require.Equal(t, candidateGenesis.Hash, manifest.GenesisHash)

			localGenesis := local.chain.Genesis.GenesisHeader()
			localGenesis.StateRoot = local.genesisRoot
			localGenesis.ComputeHash()
			require.NotEqual(t, types.ZeroHash, localGenesis.Hash)
			require.NotEqual(t, localGenesis.Hash, manifest.GenesisHash)

			states := []struct {
				name   string
				height int
			}{
				{name: "absent", height: -2},
				{name: "empty_database", height: -1},
				{name: "genesis_only", height: 0},
				{name: "existing_history", height: 4},
			}

			for _, state := range states {
				for _, required := range []bool{false, true} {
					t.Run(fmt.Sprintf(
						"%s/required=%t", state.name, required,
					), func(t *testing.T) {
						instance := local.newServer(t, required)
						local.seedLocal(t, instance, state.height)

						var logs bytes.Buffer
						instance.logger = hclog.New(&hclog.LoggerOptions{
							Name:   "snapshot-genesis-identity",
							Level:  hclog.Warn,
							Output: &logs,
						})

						// The candidate is valid under the existing configured
						// header-field checks. Only the local identity differs.
						require.NoError(t, instance.validateSnapshotManifestForNode(
							manifest,
							manifest.SnapshotKind,
						))

						number, _, identity, err := instance.localBlockchainHead()
						require.NoError(t, err)
						if state.height >= 0 {
							require.Equal(t, uint64(state.height), number)
							require.NotNil(t, identity)
							require.Equal(t, localGenesis.Hash, *identity)
						} else {
							require.Zero(t, number)
							require.Nil(t, identity)
						}

						beforeLocal := snapshotStartupFiles(
							t, datadir.ChainData(instance.config.DataDir),
						)
						beforeSource := snapshotStartupFiles(
							t, candidate.snapshotRoot,
						)

						activated, err := instance.activateConfiguredSnapshot()

						// Recent history makes bootstrap optional even when
						// the configured required flag is set. Genesis-only
						// storage still follows the configured requirement.
						wantError := state.height == 0 && required
						if wantError {
							require.ErrorContains(
								t, err,
								"does not match existing local genesis hash",
							)
						} else {
							require.NoError(t, err)
						}

						// Fresh storage keeps the established source-trust
						// policy, including full-mode opt-in requirements.
						wantActivation := state.height < 0 &&
							(mode == "recent" || required)

						if wantActivation {
							require.NotNil(t, activated)
							require.Equal(t, manifest.GenesisHash, activated.GenesisHash)
							require.Equal(t, manifest.BlockHash, activated.BlockHash)
						} else {
							require.Nil(t, activated)
							require.Equal(t, beforeLocal, snapshotStartupFiles(
								t, datadir.ChainData(instance.config.DataDir),
							))
						}

						wantMismatch := state.height >= 0 &&
							(mode == "recent" || (required && state.height == 0))
						if wantMismatch {
							require.Contains(
								t, logs.String(),
								"does not match existing local genesis hash",
							)
							require.Contains(t, logs.String(), manifest.GenesisHash.String())
							require.Contains(t, logs.String(), localGenesis.Hash.String())
						}

						require.Equal(t, beforeSource, snapshotStartupFiles(
							t, candidate.snapshotRoot,
						))
						require.NoDirExists(t, filepath.Join(
							instance.config.DataDir, snapshotActivationStageName,
						))
						require.NoDirExists(t, filepath.Join(
							instance.config.DataDir, snapshotActivationBackupName,
						))
						require.NoFileExists(t, filepath.Join(
							instance.config.DataDir, snapshotActivationMarkerName,
						))

						number, usable, identity, err := instance.localBlockchainHead()
						require.NoError(t, err)

						switch {
						case wantActivation:
							require.Equal(t, uint64(5), number)
							require.True(t, usable)
							require.NotNil(t, identity)
							require.Equal(t, manifest.GenesisHash, *identity)
						case state.height >= 0:
							require.Equal(t, uint64(state.height), number)
							require.Equal(t, state.height > 0, usable)
							require.NotNil(t, identity)
							require.Equal(t, localGenesis.Hash, *identity)
						default:
							require.Zero(t, number)
							require.False(t, usable)
							require.Nil(t, identity)
						}
					})
				}
			}
		})
	}
}

/* FINISH ALPHA CHANGE 3214 -- prevent cross-genesis snapshot replacement while preserving fresh bootstrap policy -- */
