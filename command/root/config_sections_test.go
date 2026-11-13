/* START ALPHA CHANGE 3309 -- reject null server sections through actual CLI execution -- */
package root

/* START ALPHA CHANGE 3518 -- support actual peer-directory malformed-genesis startup regressions -- */
import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"
	libp2pCrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

/* FINISH ALPHA CHANGE 3518 -- support actual peer-directory malformed-genesis startup regressions -- */

func TestConfigSections_CLI(t *testing.T) {
	// Generate a valid current genesis once so the configurations below
	// differ from a usable startup configuration only in the null section.
	genesisDir := t.TempDir()
	genesisPath := filepath.Join(genesisDir, "genesis.json")

	_, stderr, err := commandExecutionExitRun(
		t, genesisDir, false,
		"genesis", "--dir", genesisPath,
		"--validators", commandExecutionExitValidator(t),
	)
	require.NoError(t, err, "genesis fixture failed:\n%s", stderr)
	require.FileExists(t, genesisPath)

	formats := []struct {
		name      string
		suffix    string
		nullValue string
	}{
		{"json", ".json", "null"},
		{"yaml_null", ".yaml", "null"},
		{"yaml_tilde", ".yaml", "~"},
		{"yml_empty", ".yml", ""},
	}

	for _, jsonOutput := range []bool{false, true} {
		for _, format := range formats {
			for _, section := range []string{"network", "telemetry", "tx_pool"} {
				t.Run(fmt.Sprintf(
					"%s/%s/json=%t", format.name, section, jsonOutput,
				), func(t *testing.T) {
					dir := t.TempDir()
					dataDir := filepath.Join(dir, "runtime-data")
					configPath := filepath.Join(dir, "server"+format.suffix)

					document := map[string]any{
						"chain_config": genesisPath,
						"data_dir":     dataDir,
						"grpc_addr":    "127.0.0.1:0",
						"jsonrpc_addr": "127.0.0.1:0",
						"network": map[string]any{
							"libp2p_addr": "127.0.0.1:0",
						},
						"telemetry": map[string]any{},
						"tx_pool":   map[string]any{},
					}
					document[section] = nil

					var encoded []byte
					var encodeErr error

					if format.suffix == ".json" {
						encoded, encodeErr = json.Marshal(document)
					} else {
						encoded, encodeErr = yaml.Marshal(document)
					}
					require.NoError(t, encodeErr)

					if format.nullValue != "null" {
						original := section + ": null\n"
						require.Contains(t, string(encoded), original)
						encoded = []byte(strings.Replace(
							string(encoded),
							original,
							section+": "+format.nullValue+"\n",
							1,
						))
					}

					require.NoError(t, os.WriteFile(configPath, encoded, 0600))

					stdout, stderr, runErr := commandExecutionExitRun(
						t, dir, jsonOutput,
						"server", "--config", configPath,
					)

					var exitError *exec.ExitError
					require.True(t, errors.As(runErr, &exitError),
						"expected command failure; err=%v\nstdout:\n%s\nstderr:\n%s",
						runErr, stdout, stderr)
					require.Equal(t, 1, exitError.ExitCode())

					combined := stdout + stderr
					require.Contains(t, combined, fmt.Sprintf(
						"invalid server configuration: section %q must not be null",
						section,
					))
					require.NotContains(t, combined, "panic:")
					require.NoDirExists(t, dataDir)

					// The configuration must be the only entry in the working
					// directory; initialization must not create runtime output.
					entries, readErr := os.ReadDir(dir)
					require.NoError(t, readErr)
					require.Len(t, entries, 1)
					require.Equal(t, filepath.Base(configPath), entries[0].Name())
				})
			}
		}
	}
}

/* FINISH ALPHA CHANGE 3309 -- reject null server sections through actual CLI execution -- */

/* START ALPHA CHANGE 3546 -- verify positive chain ID policy through actual genesis and server commands -- */
func TestChainIDPolicy_GenesisCLI(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		for _, flag := range []string{"--chain-id", "--chainid"} {
			t.Run(fmt.Sprintf("%s/json=%t", flag, jsonOutput), func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "genesis.json")

				stdout, stderr, runErr := commandExecutionExitRun(
					t, dir, jsonOutput,
					"genesis", "--dir", path,
					flag, "0",
				)

				var exitError *exec.ExitError
				require.ErrorAs(t, runErr, &exitError)
				require.Equal(t, 1, exitError.ExitCode())

				combined := stdout + stderr
				require.Contains(t, combined,
					"--chain-id must be between 1 and 9223372036854775807, got 0")
				require.NotContains(t, combined, "panic:")
				require.NotContains(t, stdout, "[GENESIS SUCCESS]")
				require.NoFileExists(t, path)

				entries, err := os.ReadDir(dir)
				require.NoError(t, err)
				require.Empty(t, entries)
			})
		}
	}
}

func TestChainIDPolicy_ServerCLI(t *testing.T) {
	genesisDir := t.TempDir()
	generatedPath := filepath.Join(genesisDir, "genesis.json")

	_, stderr, err := commandExecutionExitRun(
		t, genesisDir, false,
		"genesis", "--dir", generatedPath,
		"--validators", commandExecutionExitValidator(t),
	)
	require.NoError(t, err, "genesis generation failed:\n%s", stderr)

	original, err := os.ReadFile(generatedPath)
	require.NoError(t, err)

	cases := []struct {
		name  string
		value any
		omit  bool
	}{
		{name: "omitted", omit: true},
		{name: "explicit_null"},
		{name: "zero", value: json.Number("0")},
		{name: "negative", value: json.Number("-1")},
	}

	for _, jsonOutput := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/json=%t", tc.name, jsonOutput), func(t *testing.T) {
				var document map[string]any
				decoder := json.NewDecoder(strings.NewReader(string(original)))
				decoder.UseNumber()
				require.NoError(t, decoder.Decode(&document))

				chainParams, ok := document["params"].(map[string]any)
				require.True(t, ok)

				if tc.omit {
					delete(chainParams, "chainID")
				} else {
					chainParams["chainID"] = tc.value
				}

				candidate, err := json.Marshal(document)
				require.NoError(t, err)

				dir := t.TempDir()
				chainPath := filepath.Join(dir, "genesis.json")
				dataPath := filepath.Join(dir, "runtime-data")
				require.NoError(t, os.WriteFile(chainPath, candidate, 0600))

				stdout, stderr, runErr := commandExecutionExitRun(
					t, dir, jsonOutput,
					"server",
					"--chain", chainPath,
					"--data-dir", dataPath,
					"--grpc-address", "127.0.0.1:0",
					"--jsonrpc", "127.0.0.1:0",
					"--libp2p", "127.0.0.1:0",
					"--history-mode", "full",
				)

				var exitError *exec.ExitError
				require.ErrorAs(t, runErr, &exitError)
				require.Equal(t, 1, exitError.ExitCode())
				require.Contains(t, stdout+stderr, "chain ID must be positive")
				require.NotContains(t, stdout+stderr, "panic:")
				require.NoDirExists(t, dataPath)

				onDisk, err := os.ReadFile(chainPath)
				require.NoError(t, err)
				require.Equal(t, candidate, onDisk)

				entries, err := os.ReadDir(dir)
				require.NoError(t, err)
				require.Len(t, entries, 1)
				require.Equal(t, "genesis.json", entries[0].Name())
			})
		}
	}

	unchanged, err := os.ReadFile(generatedPath)
	require.NoError(t, err)
	require.Equal(t, original, unchanged)
}

/* FINISH ALPHA CHANGE 3546 -- verify positive chain ID policy through actual genesis and server commands -- */

/* START ALPHA CHANGE 3342 -- verify native-token defaults and controlled CLI startup rejection -- */

func TestNativeTokenConfig_CLI(t *testing.T) {
	genesisDir := t.TempDir()
	generatedPath := filepath.Join(genesisDir, "genesis.json")

	// Omit --native-token-config so the real generator supplies defaults.
	_, stderr, err := commandExecutionExitRun(
		t, genesisDir, false,
		"genesis", "--dir", generatedPath,
		"--validators", commandExecutionExitValidator(t),
	)
	require.NoError(t, err, "genesis generation failed:\n%s", stderr)

	original, err := os.ReadFile(generatedPath)
	require.NoError(t, err)

	for _, jsonOutput := range []bool{false, true} {
		for _, mode := range []string{"omitted", "explicit_null"} {
			t.Run(fmt.Sprintf("%s/json=%t", mode, jsonOutput), func(t *testing.T) {
				// Preserve numeric values exactly while modifying only
				// the required native-token section in a temporary copy.
				var document map[string]any
				decoder := json.NewDecoder(strings.NewReader(string(original)))
				decoder.UseNumber()
				require.NoError(t, decoder.Decode(&document))

				params, ok := document["params"].(map[string]any)
				require.True(t, ok)
				engines, ok := params["engine"].(map[string]any)
				require.True(t, ok)
				engine, ok := engines["innbcbft"].(map[string]any)
				require.True(t, ok)

				token, ok := engine["nativeTokenConfig"].(map[string]any)
				require.True(t, ok)
				require.Equal(t, "INNBC Chain", token["name"])
				require.Equal(t, "INNBC", token["symbol"])
				require.Equal(t, json.Number("18"), token["decimals"])

				if mode == "omitted" {
					delete(engine, "nativeTokenConfig")
				} else {
					engine["nativeTokenConfig"] = nil
				}

				candidate, err := json.Marshal(document)
				require.NoError(t, err)

				dir := t.TempDir()
				chainPath := filepath.Join(dir, "genesis.json")
				dataPath := filepath.Join(dir, "runtime-data")
				require.NoError(t, os.WriteFile(chainPath, candidate, 0600))

				stdout, stderr, runErr := commandExecutionExitRun(
					t, dir, jsonOutput,
					"server",
					"--chain", chainPath,
					"--data-dir", dataPath,
					"--grpc-address", "127.0.0.1:0",
					"--jsonrpc", "127.0.0.1:0",
					"--libp2p", "127.0.0.1:0",
					"--history-mode", "full",
				)

				commandExecutionExitFailure(
					t, stdout, stderr, runErr, jsonOutput,
					"invalid INNBCBFT configuration: nativeTokenConfig is required and must not be null",
				)
				require.NotContains(t, stdout+stderr, "panic:")
				require.NoDirExists(t, dataPath)

				onDisk, readErr := os.ReadFile(chainPath)
				require.NoError(t, readErr)
				require.Equal(t, candidate, onDisk)

				entries, readErr := os.ReadDir(dir)
				require.NoError(t, readErr)
				require.Len(t, entries, 1)
				require.Equal(t, "genesis.json", entries[0].Name())
			})
		}
	}

	unchanged, err := os.ReadFile(generatedPath)
	require.NoError(t, err)
	require.Equal(t, original, unchanged)
}

/* FINISH ALPHA CHANGE 3342 -- verify native-token defaults and controlled CLI startup rejection -- */

/* START ALPHA CHANGE 3443 -- verify genesis and server epoch validation through real CLI subprocesses -- */

func TestEpochSize_GenesisCLI(t *testing.T) {
	validator := commandExecutionExitValidator(t)

	for _, jsonOutput := range []bool{false, true} {
		for _, epochSize := range []uint64{0, 1, 2, 30} {
			t.Run(fmt.Sprintf("epoch=%d/json=%t", epochSize, jsonOutput), func(t *testing.T) {
				dir := t.TempDir()
				path := filepath.Join(dir, "genesis.json")

				stdout, stderr, runErr := commandExecutionExitRun(
					t, dir, jsonOutput,
					"genesis", "--dir", path,
					"--validators", validator,
					"--epoch-size", fmt.Sprint(epochSize),
				)

				require.NotContains(t, stdout+stderr, "panic:")

				if epochSize < 2 {
					var exitError *exec.ExitError
					require.ErrorAs(t, runErr, &exitError)
					require.Equal(t, 1, exitError.ExitCode())
					require.Contains(t, stdout+stderr,
						"epoch size must be greater than 1")
					require.NotContains(t, stdout, "[GENESIS SUCCESS]")
					require.NoFileExists(t, path)

					entries, err := os.ReadDir(dir)
					require.NoError(t, err)
					require.Empty(t, entries)
					return
				}

				require.NoError(t, runErr,
					"stdout:\n%s\nstderr:\n%s", stdout, stderr)

				content, err := os.ReadFile(path)
				require.NoError(t, err)

				var document struct {
					Params struct {
						Engine map[string]struct {
							EpochSize uint64 `json:"epochSize"`
						} `json:"engine"`
					} `json:"params"`
				}
				require.NoError(t, json.Unmarshal(content, &document))

				engine, ok := document.Params.Engine["innbcbft"]
				require.True(t, ok)
				require.Equal(t, epochSize, engine.EpochSize)
			})
		}
	}
}

func TestEpochSize_ServerCLI(t *testing.T) {
	genesisDir := t.TempDir()
	generatedPath := filepath.Join(genesisDir, "genesis.json")

	_, stderr, err := commandExecutionExitRun(
		t, genesisDir, false,
		"genesis", "--dir", generatedPath,
		"--validators", commandExecutionExitValidator(t),
		"--epoch-size", "2",
	)
	require.NoError(t, err, "genesis generation failed:\n%s", stderr)

	original, err := os.ReadFile(generatedPath)
	require.NoError(t, err)

	cases := []struct {
		name  string
		value any
		omit  bool
	}{
		{name: "omitted", omit: true},
		{name: "explicit_null"},
		{name: "zero", value: json.Number("0")},
		{name: "one", value: json.Number("1")},
	}

	for _, jsonOutput := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("%s/json=%t", tc.name, jsonOutput), func(t *testing.T) {
				// Alter only a temporary copy, preserving full-width numbers.
				var document map[string]any
				decoder := json.NewDecoder(strings.NewReader(string(original)))
				decoder.UseNumber()
				require.NoError(t, decoder.Decode(&document))

				chainParams, ok := document["params"].(map[string]any)
				require.True(t, ok)
				engines, ok := chainParams["engine"].(map[string]any)
				require.True(t, ok)
				engine, ok := engines["innbcbft"].(map[string]any)
				require.True(t, ok)

				if tc.omit {
					delete(engine, "epochSize")
				} else {
					engine["epochSize"] = tc.value
				}

				candidate, err := json.Marshal(document)
				require.NoError(t, err)

				dir := t.TempDir()
				chainPath := filepath.Join(dir, "genesis.json")
				dataPath := filepath.Join(dir, "runtime-data")
				require.NoError(t, os.WriteFile(chainPath, candidate, 0600))

				stdout, stderr, runErr := commandExecutionExitRun(
					t, dir, jsonOutput,
					"server",
					"--chain", chainPath,
					"--data-dir", dataPath,
					"--grpc-address", "127.0.0.1:0",
					"--jsonrpc", "127.0.0.1:0",
					"--libp2p", "127.0.0.1:0",
					"--history-mode", "full",
				)

				commandExecutionExitFailure(
					t, stdout, stderr, runErr, jsonOutput,
					"invalid INNBCBFT configuration: epochSize must be at least 2",
				)
				require.NotContains(t, stdout+stderr, "panic:")
				require.NoDirExists(t, dataPath)

				onDisk, err := os.ReadFile(chainPath)
				require.NoError(t, err)
				require.Equal(t, candidate, onDisk)

				entries, err := os.ReadDir(dir)
				require.NoError(t, err)
				require.Len(t, entries, 1)
				require.Equal(t, "genesis.json", entries[0].Name())
			})
		}
	}

	unchanged, err := os.ReadFile(generatedPath)
	require.NoError(t, err)
	require.Equal(t, original, unchanged)
}

/* FINISH ALPHA CHANGE 3443 -- verify genesis and server epoch validation through real CLI subprocesses -- */

/* START ALPHA CHANGE 3518 -- reject null genesis validators before peer-directory startup side effects -- */
func TestPeerDirectoryGenesisValidators_CLI(t *testing.T) {
	privateKey, _, err := libp2pCrypto.GenerateEd25519Key(rand.Reader)
	require.NoError(t, err)
	id, err := peer.IDFromPrivateKey(privateKey)
	require.NoError(t, err)

	/* START ALPHA CHANGE 3849 -- replace the shared fixture peer identity without duplicating its peer component -- */
	// Preserve the valid BLS fields and use this test's generated peer ID.
	_, validatorFields, ok := strings.Cut(commandExecutionExitValidator(t), ":")
	require.True(t, ok)
	validatorArgument := "/ip4/127.0.0.1/tcp/30301/p2p/" + id.String() + ":" + validatorFields
	/* FINISH ALPHA CHANGE 3849 -- replace the shared fixture peer identity without duplicating its peer component -- */

	genesisDir := t.TempDir()
	generatedPath := filepath.Join(genesisDir, "genesis.json")
	_, stderr, err := commandExecutionExitRun(
		t, genesisDir, false,
		"genesis",
		"--dir", generatedPath,
		"--validators", validatorArgument,
	)
	require.NoError(t, err, "valid genesis generation failed:\n%s", stderr)

	original, err := os.ReadFile(generatedPath)
	require.NoError(t, err)

	for _, jsonOutput := range []bool{false, true} {
		for _, nilIndex := range []int{0, 1} {
			t.Run(fmt.Sprintf("index=%d/json=%t", nilIndex, jsonOutput), func(t *testing.T) {
				var document map[string]any
				decoder := json.NewDecoder(strings.NewReader(string(original)))
				decoder.UseNumber()
				require.NoError(t, decoder.Decode(&document))

				params, ok := document["params"].(map[string]any)
				require.True(t, ok)
				engines, ok := params["engine"].(map[string]any)
				require.True(t, ok)
				engine, ok := engines[innbcbft.ConsensusName].(map[string]any)
				require.True(t, ok)
				validators, ok := engine["initialValidatorSet"].([]any)
				require.True(t, ok)
				require.Len(t, validators, 1)

				if nilIndex == 0 {
					engine["initialValidatorSet"] = []any{nil}
				} else {
					engine["initialValidatorSet"] = []any{validators[0], nil}
				}

				candidate, err := json.Marshal(document)
				require.NoError(t, err)

				dir := t.TempDir()
				chainPath := filepath.Join(dir, "genesis.json")
				cacheDir := filepath.Join(dir, "must-not-create")
				cachePath := filepath.Join(cacheDir, "peers.json")
				require.NoError(t, os.WriteFile(chainPath, candidate, 0600))

				// Prove that the shared loader accepts all preceding
				// configuration requirements and retains the null entry.
				loaded, err := innbcbft.LoadINNBCBFTConfig(chainPath)
				require.NoError(t, err)
				require.Len(t, loaded.InitialValidatorSet, nilIndex+1)
				require.Nil(t, loaded.InitialValidatorSet[nilIndex])

				// Reserve the listen address. If startup proceeds past
				// validation unexpectedly, binding fails instead of
				// starting a long-running directory server.
				listener, err := net.Listen("tcp", "127.0.0.1:0")
				require.NoError(t, err)
				t.Cleanup(func() {
					_ = listener.Close()
				})

				stdout, stderr, runErr := commandExecutionExitRun(
					t, dir, jsonOutput,
					"peerdirectory",
					"--chain", chainPath,
					"--jsonrpc", "http://127.0.0.1:1",
					"--validator-registry", "0x0000000000000000000000000000000000000107",
					"--listen", listener.Addr().String(),
					"--cache-file", cachePath,
				)

				var exitError *exec.ExitError
				require.True(t, errors.As(runErr, &exitError),
					"expected CLI failure; err=%v\nstdout:\n%s\nstderr:\n%s",
					runErr, stdout, stderr)
				require.Equal(t, 1, exitError.ExitCode())

				combined := stdout + stderr
				require.Contains(t, combined,
					fmt.Sprintf("nil genesis validator at index %d", nilIndex))
				require.NotContains(t, combined, "panic:")
				require.NotContains(t, combined, "failed to parse genesis validator multiaddr")
				require.NoDirExists(t, cacheDir)
				require.NoFileExists(t, cachePath)

				onDisk, err := os.ReadFile(chainPath)
				require.NoError(t, err)
				require.Equal(t, candidate, onDisk)

				entries, err := os.ReadDir(dir)
				require.NoError(t, err)
				require.Len(t, entries, 1)
				require.Equal(t, "genesis.json", entries[0].Name())
			})
		}
	}

	unchanged, err := os.ReadFile(generatedPath)
	require.NoError(t, err)
	require.Equal(t, original, unchanged)
}

/* FINISH ALPHA CHANGE 3518 -- reject null genesis validators before peer-directory startup side effects -- */
