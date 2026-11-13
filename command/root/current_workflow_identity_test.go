/* START ALPHA CHANGE 3238 -- verify current node provisioning and public registration identities -- */
package root

import (
	/* START ALPHA CHANGE 3364 -- verify structured public-only inspection output -- */
	"encoding/hex"
	"encoding/json"
	"fmt"

	/* FINISH ALPHA CHANGE 3364 -- verify structured public-only inspection output -- */
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/innovativebioresearch/innbc-chain/network"
	secretshelper "github.com/innovativebioresearch/innbc-chain/secrets/helper"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
)

func TestCurrentWorkflow_Identity(t *testing.T) {
	for _, mode := range []string{"private-local", "external-validator"} {
		t.Run(mode, func(t *testing.T) {
			f := cwProvision(t, 2, mode)

			operators := make(map[string]bool)
			peerIDs := make(map[string]bool)
			blsKeys := make(map[string]bool)

			for index, node := range f.nodes {
				number := index + 1
				envPath := filepath.Join(f.config, fmt.Sprintf("node%d.env", number))

				info, err := os.Stat(envPath)
				require.NoError(t, err)
				require.Equal(t, os.FileMode(0600), info.Mode().Perm())

				require.Equal(t, f.chain, node.env["INNBC_CHAIN"])
				require.Equal(t, node.dir, node.env["INNBC_DATA_DIR"])
				require.Equal(t, fmt.Sprintf("127.0.0.1:%d", 8544+number),
					node.env["INNBC_JSONRPC_ADDR"])
				require.Equal(t, fmt.Sprintf("127.0.0.1:%d", 9631+number),
					node.env["INNBC_GRPC_ADDR"])
				require.Equal(t, "true", node.env["INNBC_PEER_DIRECTORY_ENABLED"])
				require.Equal(t, "false", node.env["INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED"])

				host := "127.0.0.1"
				directory := "http://127.0.0.1:8080/api/v1/peers/exchange"
				if mode == "external-validator" {
					host = "0.0.0.0"
					directory = "https://innbcchain.io/api/v1/peers/exchange"
				}
				require.Equal(t, fmt.Sprintf("%s:%d", host, 30300+number),
					node.env["INNBC_LIBP2P_ADDR"])
				require.Equal(t, directory, node.env["INNBC_PEER_DIRECTORY_URL"])
				require.Empty(t, node.env["INNBC_NAT_ADDR"])
				require.Empty(t, node.env["INNBC_DNS_ADDR"])

				if number == 1 {
					require.Equal(t, "full", node.env["INNBC_HISTORY_MODE"])
					require.Equal(t, "1024", node.env["INNBC_MDBX_MAP_SIZE_GB"])
					require.Equal(t, filepath.Join(f.snapshots, "checkpoint"),
						node.env["INNBC_SNAPSHOT_SOURCE_DIR"])
					require.NotContains(t, node.env, "INNBC_HISTORY_RETENTION_BLOCKS")
					require.NotContains(t, node.env, "INNBC_HISTORY_SEGMENT_BLOCKS")
				} else {
					require.Equal(t, "recent", node.env["INNBC_HISTORY_MODE"])
					require.Equal(t, "128", node.env["INNBC_MDBX_MAP_SIZE_GB"])
					require.Equal(t, "43200", node.env["INNBC_HISTORY_RETENTION_BLOCKS"])
					require.Equal(t, "1800", node.env["INNBC_HISTORY_SEGMENT_BLOCKS"])
					require.Equal(t, filepath.Join(f.snapshots, "segmented"),
						node.env["INNBC_SNAPSHOT_SOURCE_DIR"])
				}

				manager, err := secretshelper.SetupLocalSecretsManager(node.dir)
				require.NoError(t, err)

				account, err := wallet.NewAccountFromSecret(manager)
				require.NoError(t, err)
				require.Equal(t, account.Address().String(), node.info[cwOperatorField])
				/* START ALPHA CHANGE 3241 -- match the prefixed BLS public key printed by node info -- */
				require.Equal(t, "0x"+hex.EncodeToString(account.Bls.PublicKey().Marshal()),
					node.info[cwBLSField])
				/* FINISH ALPHA CHANGE 3241 -- match the prefixed BLS public key printed by node info -- */

				networkKey, err := network.ReadLibp2pKey(manager)
				require.NoError(t, err)
				peerID, err := peer.IDFromPrivateKey(networkKey)
				require.NoError(t, err)
				require.Equal(t, peerID.String(), node.info[cwPeerField])

				/* START ALPHA CHANGE 3241 -- decode the prefixed public BLS key from node info -- */
				publicKey, err := hex.DecodeString(
					strings.TrimPrefix(node.info[cwBLSField], "0x"),
				)
				require.NoError(t, err)
				/* FINISH ALPHA CHANGE 3241 -- decode the prefixed public BLS key from node info -- */
				/* START ALPHA CHANGE 3241 -- validate and decode the prefixed public BLS proof from node info -- */
				require.True(t, strings.HasPrefix(node.info[cwProofField], "0x"),
					"node info must print the BLS proof with its hexadecimal prefix")

				proof, err := hex.DecodeString(
					strings.TrimPrefix(node.info[cwProofField], "0x"),
				)
				require.NoError(t, err)
				/* FINISH ALPHA CHANGE 3241 -- validate and decode the prefixed public BLS proof from node info -- */
				_, err = signer.VerifyINNBCBLSPoP(publicKey, account.Address(), proof)
				require.NoError(t, err)

				require.False(t, operators[node.info[cwOperatorField]],
					"duplicate operator")
				require.False(t, peerIDs[node.info[cwPeerField]], "duplicate peer ID")
				require.False(t, blsKeys[node.info[cwBLSField]], "duplicate BLS key")
				operators[node.info[cwOperatorField]] = true
				peerIDs[node.info[cwPeerField]] = true
				blsKeys[node.info[cwBLSField]] = true

				/* START ALPHA CHANGE 3364 -- inspect the selected identity and its actual environment -- */
				output := cwRequireCLI(
					t, f.dir, "node", "info", fmt.Sprintf("node%d", number),
					"--base-dir", f.base,
					"--config-dir", f.config,
				)
				/* FINISH ALPHA CHANGE 3364 -- inspect the selected identity and its actual environment -- */
				for _, secret := range node.keys {
					require.False(t, strings.Contains(output, string(secret)),
						"node info exposed a private key")
				}

				cwAssertKeysUnchanged(t, node)
				require.NoDirExists(t, datadir.ChainData(node.dir))
			}

			require.NoFileExists(t, f.chain)
			require.NoDirExists(t, f.snapshots)

			// Creating the next node must preserve existing identities and env files.
			firstEnv, err := os.ReadFile(filepath.Join(f.config, "node1.env"))
			require.NoError(t, err)

			cwRequireCLI(t, f.dir, f.createArgs(1, mode)...)
			require.FileExists(t, filepath.Join(f.config, "node3.env"))
			require.FileExists(t, datadir.ValidatorKey(filepath.Join(f.base, "node3")))

			after, err := os.ReadFile(filepath.Join(f.config, "node1.env"))
			require.NoError(t, err)
			require.Equal(t, string(firstEnv), string(after))

			for _, node := range f.nodes {
				cwAssertKeysUnchanged(t, node)
			}
		})
	}
}

/* START ALPHA CHANGE 3322 -- reject the retired genesis drift flag through the actual CLI -- */
func TestChainClock_GenesisRejectsRetiredFlag(t *testing.T) {
	for _, jsonMode := range []bool{false, true} {
		t.Run(fmt.Sprint(jsonMode), func(t *testing.T) {
			f := cwPaths(t)
			args := []string{"genesis", "--block-time-drift", "10"}
			if jsonMode {
				args = append(args, "--json")
			}

			_, err := cwCLI(t, f.dir, args...)
			require.ErrorContains(t, err, "unknown flag: --block-time-drift")
		})
	}
}

/* FINISH ALPHA CHANGE 3322 -- reject the retired genesis drift flag through the actual CLI -- */

func TestCurrentWorkflow_InvalidCreate(t *testing.T) {
	/* START ALPHA CHANGE 3358 -- reject invalid batch counts independently from supported node numbers -- */
	for _, count := range []string{
		"0", "-1", "11", "invalid",
		"9999999999999999999999999999999999999999",
	} {
		/* FINISH ALPHA CHANGE 3358 -- reject invalid batch counts independently from supported node numbers -- */
		t.Run(count, func(t *testing.T) {
			f := cwPaths(t)
			args := f.createArgs(1, "private-local")
			args[2] = count

			_, err := cwCLI(t, f.dir, args...)
			require.Error(t, err)
			require.NoDirExists(t, f.base)
			require.NoDirExists(t, f.config)
		})
	}

	t.Run("unsupported_mode", func(t *testing.T) {
		f := cwPaths(t)
		_, err := cwCLI(t, f.dir, f.createArgs(1, "unsupported")...)
		require.Error(t, err)
		require.NoDirExists(t, f.base)
		require.NoDirExists(t, f.config)
	})

	/* START ALPHA CHANGE 3358 -- skip environment-only reservations while preserving their contents -- */
	t.Run("environment_only_reservation_is_skipped", func(t *testing.T) {
		f := cwPaths(t)
		require.NoError(t, os.MkdirAll(f.config, 0755))

		envPath := filepath.Join(f.config, "node1.env")
		original := "INNBC_TEST_SENTINEL=preserve\n"
		require.NoError(t, os.WriteFile(envPath, []byte(original), 0600))

		cwRequireCLI(t, f.dir, f.createArgs(1, "private-local")...)

		actual, err := os.ReadFile(envPath)
		require.NoError(t, err)
		require.Equal(t, original, string(actual))
		require.NoDirExists(t, filepath.Join(f.base, "node1"))
		require.FileExists(t, datadir.ValidatorKey(filepath.Join(f.base, "node2")))
		require.FileExists(t, filepath.Join(f.config, "node2.env"))
		require.NoFileExists(t, f.chain)
	})
	/* FINISH ALPHA CHANGE 3358 -- skip environment-only reservations while preserving their contents -- */
}

/* FINISH ALPHA CHANGE 3238 -- verify current node provisioning and public registration identities -- */

/* START ALPHA CHANGE 3358 -- verify extended node numbering and capacity through actual CLI subprocesses -- */

func TestCurrentWorkflow_NodeAllocation(t *testing.T) {
	for _, mode := range []string{"private-local", "external-validator"} {
		t.Run(mode, func(t *testing.T) {
			f := cwPaths(t)
			require.NoError(t, os.MkdirAll(f.base, 0750))

			for number := 1; number <= 10; number++ {
				require.NoError(t, os.Mkdir(
					filepath.Join(f.base, fmt.Sprintf("node%d", number)), 0700,
				))
			}

			cwRequireCLI(t, f.dir, f.createArgs(2, mode)...)

			for _, number := range []int{11, 12} {
				name := fmt.Sprintf("node%d", number)
				dir := filepath.Join(f.base, name)
				keys := cwReadKeys(t, dir)

				/* START ALPHA CHANGE 3364 -- validate extended identities against their matching environments -- */
				info := cwRequireCLI(
					t, f.dir, "node", "info", name,
					"--base-dir", f.base,
					"--config-dir", f.config,
				)
				/* FINISH ALPHA CHANGE 3364 -- validate extended identities against their matching environments -- */
				require.Contains(t, info, "Name|"+name)
				require.Contains(t, info, cwOperatorField+"|")
				require.Contains(t, info, cwBLSField+"|")
				require.Contains(t, info, cwProofField+"|")
				require.Contains(t, info, cwPeerField+"|")
				require.Equal(t, keys, cwReadKeys(t, dir))

				env := cwReadEnv(t, filepath.Join(f.config, name+".env"))
				require.Equal(t, fmt.Sprintf("127.0.0.1:%d", 8544+number),
					env["INNBC_JSONRPC_ADDR"])
				require.Equal(t, fmt.Sprintf("127.0.0.1:%d", 9631+number),
					env["INNBC_GRPC_ADDR"])
				require.Equal(t, "recent", env["INNBC_HISTORY_MODE"])

				host := "127.0.0.1"
				if mode == "external-validator" {
					host = "0.0.0.0"
				}
				require.Equal(t, fmt.Sprintf("%s:%d", host, 30300+number),
					env["INNBC_LIBP2P_ADDR"])

				for _, secret := range keys {
					require.NotContains(t, info, string(secret))
				}
			}

			/* START ALPHA CHANGE 3364 -- preserve invalid-number rejection with explicit inspection paths -- */
			_, err := cwCLI(
				t, f.dir, "node", "info", "node101",
				"--base-dir", f.base,
				"--config-dir", f.config,
			)
			/* FINISH ALPHA CHANGE 3364 -- preserve invalid-number rejection with explicit inspection paths -- */
			require.ErrorContains(t, err, "node1 to node100")
			require.NoDirExists(t, filepath.Join(f.base, "node101"))
			require.NoDirExists(t, f.snapshots)
			require.NoFileExists(t, f.chain)
		})
	}
}

func TestCurrentWorkflow_NodeCapacity(t *testing.T) {
	f := cwPaths(t)
	require.NoError(t, os.MkdirAll(f.base, 0750))

	for number := 1; number < 100; number++ {
		require.NoError(t, os.Mkdir(
			filepath.Join(f.base, fmt.Sprintf("node%d", number)), 0700,
		))
	}

	_, err := cwCLI(t, f.dir, f.createArgs(2, "private-local")...)
	require.ErrorContains(t, err, "only 1 available slot remains")
	require.NoDirExists(t, f.config)
	require.NoDirExists(t, filepath.Join(f.base, "node100"))

	cwRequireCLI(t, f.dir, f.createArgs(1, "private-local")...)
	dir := filepath.Join(f.base, "node100")
	keys := cwReadKeys(t, dir)
	envPath := filepath.Join(f.config, "node100.env")
	envBefore, err := os.ReadFile(envPath)
	require.NoError(t, err)

	/* START ALPHA CHANGE 3364 -- inspect the final supported identity and environment together -- */
	info := cwRequireCLI(
		t, f.dir, "node", "info", "node100",
		"--base-dir", f.base,
		"--config-dir", f.config,
	)
	/* FINISH ALPHA CHANGE 3364 -- inspect the final supported identity and environment together -- */
	require.Contains(t, info, "Name|node100")
	require.Equal(t, keys, cwReadKeys(t, dir))

	env := cwReadEnv(t, envPath)
	require.Equal(t, "127.0.0.1:8644", env["INNBC_JSONRPC_ADDR"])
	require.Equal(t, "127.0.0.1:9731", env["INNBC_GRPC_ADDR"])
	require.Equal(t, "127.0.0.1:30400", env["INNBC_LIBP2P_ADDR"])

	_, err = cwCLI(t, f.dir, f.createArgs(1, "private-local")...)
	require.ErrorContains(t, err, "only 0 available slots remain")
	require.NoDirExists(t, filepath.Join(f.base, "node101"))

	envAfter, err := os.ReadFile(envPath)
	require.NoError(t, err)
	require.Equal(t, envBefore, envAfter)
	require.Equal(t, keys, cwReadKeys(t, dir))
}

/* FINISH ALPHA CHANGE 3358 -- verify extended node numbering and capacity through actual CLI subprocesses -- */

/* START ALPHA CHANGE 3364 -- verify public inspection and failure statuses through actual CLI subprocesses -- */

func TestCurrentWorkflow_NodeInfo(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(fmt.Sprintf("json=%t", jsonOutput), func(t *testing.T) {
			f := cwPaths(t)
			cwRequireCLI(t, f.dir, f.createArgs(3, "private-local")...)

			require.NoError(t, os.RemoveAll(filepath.Join(f.base, "node2")))
			require.NoError(t, os.Remove(filepath.Join(f.config, "node2.env")))

			node1 := filepath.Join(f.base, "node1")
			node3 := filepath.Join(f.base, "node3")
			keys1 := cwReadKeys(t, node1)
			keys3 := cwReadKeys(t, node3)
			envPath := filepath.Join(f.config, "node1.env")
			env, err := os.ReadFile(envPath)
			require.NoError(t, err)
			env = append(env, []byte(
				"\nINNBC_PRIVATE_KEY="+string(keys1[datadir.ValidatorKey(node1)])+"\n",
			)...)
			require.NoError(t, os.WriteFile(envPath, env, 0600))

			args := []string{
				"node", "info", "all",
				"--base-dir", f.base,
				"--config-dir", f.config,
			}

			stdout, stderr, err := commandExecutionExitRun(
				t, f.dir, jsonOutput, args...,
			)
			operatorExitAssertCode(t, err, 0, stdout, stderr)
			require.Empty(t, strings.TrimSpace(stderr))
			require.Contains(t, stdout, "WARNING: NODE 2 IDENTITY MISSING")
			require.NotContains(t, stdout, "INNBC_PRIVATE_KEY")

			if jsonOutput {
				var report struct {
					Nodes []struct {
						Name           string   `json:"name"`
						IdentityStatus string   `json:"identity_status"`
						Errors         []string `json:"errors"`
					} `json:"nodes"`
				}
				require.NoError(t, json.Unmarshal([]byte(stdout), &report))
				require.Len(t, report.Nodes, 3)
				require.Equal(t, "node3", report.Nodes[2].Name)
				require.Equal(t, "valid", report.Nodes[2].IdentityStatus)
				require.Empty(t, report.Nodes[1].Errors)
			} else {
				require.Contains(t, stdout, "Name|node3")
				require.NotContains(t, stdout, "Name|node4")
			}

			for _, keys := range []map[string][]byte{keys1, keys3} {
				for _, key := range keys {
					require.NotContains(t, stdout+stderr, string(key))
				}
			}
			require.Equal(t, keys1, cwReadKeys(t, node1))
			require.Equal(t, keys3, cwReadKeys(t, node3))

			// A corrupt earlier node must not prevent later-node inspection.
			canary := "PRIVATE_INSPECTION_CANARY"
			/* START ALPHA CHANGE 3367 -- restore protected key permissions after preparing the corrupt workflow fixture -- */
			keyPath := datadir.ValidatorKey(node1)
			keyInfo, err := os.Stat(keyPath)
			require.NoError(t, err)
			require.NoError(t, os.Chmod(keyPath, 0600))
			require.NoError(t, os.WriteFile(keyPath, []byte(canary), 0600))
			require.NoError(t, os.Chmod(keyPath, keyInfo.Mode().Perm()))
			/* FINISH ALPHA CHANGE 3367 -- restore protected key permissions after preparing the corrupt workflow fixture -- */
			stdout, stderr, err = commandExecutionExitRun(
				t, f.dir, jsonOutput, args...,
			)
			commandExecutionExitFailure(
				t, stdout, stderr, err, jsonOutput, "node inspection failed",
			)
			require.NotContains(t, stdout+stderr, canary)
			require.NotContains(t, stdout+stderr,
				string(keys1[datadir.ValidatorKey(node1)]))
			require.Equal(t, keys3, cwReadKeys(t, node3))

			if jsonOutput {
				var report struct {
					Nodes []struct {
						Name           string `json:"name"`
						IdentityStatus string `json:"identity_status"`
					} `json:"nodes"`
				}
				require.NoError(t, json.Unmarshal([]byte(stdout), &report))
				require.Len(t, report.Nodes, 3)
				require.Equal(t, "invalid", report.Nodes[0].IdentityStatus)
				require.Equal(t, "valid", report.Nodes[2].IdentityStatus)
			} else {
				require.Contains(t, stdout, "ERROR: NODE 1")
				require.Contains(t, stdout, "Name|node3")
			}

			actual, err := os.ReadFile(datadir.ValidatorKey(node1))
			require.NoError(t, err)
			require.Equal(t, canary, string(actual))
			actualEnv, err := os.ReadFile(envPath)
			require.NoError(t, err)
			require.Equal(t, env, actualEnv)
			require.NoDirExists(t, filepath.Join(f.base, "node2"))
			require.NoFileExists(t, filepath.Join(f.config, "node2.env"))
			require.NoDirExists(t, f.snapshots)
			require.NoFileExists(t, f.chain)
		})
	}
}

/* FINISH ALPHA CHANGE 3364 -- verify public inspection and failure statuses through actual CLI subprocesses -- */
