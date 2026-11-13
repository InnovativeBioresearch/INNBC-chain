package node

/* START ALPHA CHANGE 3357 -- test bounded allocation and exclusive identity provisioning -- */
import (
	/* START ALPHA CHANGE 3363 -- verify read-only inspection and public-only diagnostics -- */
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	/* FINISH ALPHA CHANGE 3363 -- verify read-only inspection and public-only diagnostics -- */

	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/stretchr/testify/require"
)

/* FINISH ALPHA CHANGE 3357 -- test bounded allocation and exclusive identity provisioning -- */

func TestRunPrepareCommandUsesDeterministicNodeNumbers(t *testing.T) {
	baseDir := t.TempDir()
	configDir := t.TempDir()
	snapshotRoot := filepath.Join(t.TempDir(), "snapshots")

	originalConfig := *createConfig
	t.Cleanup(func() {
		*createConfig = originalConfig
	})

	createConfig.baseDir = baseDir
	createConfig.configDir = configDir
	/* START ALPHA CHANGE 1969 -- test an independent generated snapshot root -- */
	createConfig.snapshotRoot = snapshotRoot
	/* FINISH ALPHA CHANGE 1969 -- test an independent generated snapshot root -- */
	createConfig.chainPath = "genesis.json"
	createConfig.mode = privateLocalMode

	for i := 1; i <= 6; i++ {
		require.NoError(t, os.MkdirAll(filepath.Join(baseDir, fmt.Sprintf("node%d", i)), 0700))
	}

	require.NoError(t, runPrepareCommand(nil, []string{"6"}))

	for i := 1; i <= 6; i++ {
		nodeName := fmt.Sprintf("node%d", i)
		envPath := filepath.Join(configDir, nodeName+".env")
		require.FileExists(t, envPath)
		envBytes, err := os.ReadFile(envPath)
		require.NoError(t, err)
		envContent := string(envBytes)
		require.Contains(t, envContent, fmt.Sprintf("INNBC_JSONRPC_ADDR=127.0.0.1:%d", 8544+i))
		require.Contains(t, envContent, fmt.Sprintf("INNBC_GRPC_ADDR=127.0.0.1:%d", 9631+i))
		require.Contains(t, envContent, fmt.Sprintf("INNBC_LIBP2P_ADDR=127.0.0.1:%d", 30300+i))
		/* START ALPHA CHANGE 1209 -- test generated optional advertised peer addresses -- */
		require.Contains(t, envContent, "INNBC_NAT_ADDR=")
		require.Contains(t, envContent, "INNBC_DNS_ADDR=")
		/* FINISH ALPHA CHANGE 1209 -- test generated optional advertised peer addresses -- */
		/* START ALPHA CHANGE 2497 -- test full-only historical query environment overrides -- */
		if i == 1 {
			require.Contains(t, envContent, "INNBC_JSONRPC_BLOCK_RANGE_LIMIT=256\n")
			require.Contains(t, envContent, "INNBC_JSONRPC_FEE_HISTORY_BLOCK_LIMIT=256\n")
			require.Contains(t, envContent, "INNBC_JSONRPC_FEE_HISTORY_PERCENTILE_LIMIT=10\n")
			require.Contains(t, envContent, "INNBC_JSONRPC_FEE_HISTORY_RESULT_LIMIT=2560\n")
		} else {
			require.NotContains(t, envContent, "INNBC_JSONRPC_BLOCK_RANGE_LIMIT=")
			require.NotContains(t, envContent, "INNBC_JSONRPC_FEE_HISTORY_BLOCK_LIMIT=")
			require.NotContains(t, envContent, "INNBC_JSONRPC_FEE_HISTORY_PERCENTILE_LIMIT=")
			require.NotContains(t, envContent, "INNBC_JSONRPC_FEE_HISTORY_RESULT_LIMIT=")
		}
		/* FINISH ALPHA CHANGE 2497 -- test full-only historical query environment overrides -- */
		/* START ALPHA CHANGE 1065 -- test generated node storage and snapshot defaults -- */
		if i == 1 {
			/* START ALPHA CHANGE 1739 -- test generated full-node checkpoint snapshot source -- */
			require.Contains(t, envContent, "INNBC_HISTORY_MODE=full")
			/* START ALPHA CHANGE 1769 -- exclude segmented geometry from full-node environments -- */
			require.NotContains(t, envContent, "INNBC_HISTORY_RETENTION_BLOCKS=")
			require.NotContains(t, envContent, "INNBC_HISTORY_SEGMENT_BLOCKS=")
			/* FINISH ALPHA CHANGE 1769 -- exclude segmented geometry from full-node environments -- */
			require.Contains(
				t,
				envContent,
				fmt.Sprintf(
					"INNBC_SNAPSHOT_SOURCE_DIR=%s",
					filepath.Join(
						/* START ALPHA CHANGE 1979 -- expect the independent checkpoint snapshot root -- */
						snapshotRoot,
						"checkpoint",
						/* FINISH ALPHA CHANGE 1979 -- expect the independent checkpoint snapshot root -- */
					),
				),
			)
			require.Contains(t, envContent, "INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED=false")
			require.NotContains(t, envContent, "INNBC_SNAPSHOT_INTERVAL_BLOCKS")
			require.NotContains(t, envContent, "INNBC_SNAPSHOT_DIR=")
			require.NotContains(t, envContent, "INNBC_SNAPSHOT_PRODUCER_ENABLED")
			/* START ALPHA CHANGE 3061 -- verify generated full-history nodes use one TiB -- */
			require.Contains(t, envContent, "INNBC_MDBX_MAP_SIZE_GB=1024")
			require.NotContains(t, envContent, "INNBC_MDBX_MAP_SIZE_GB=128")
			/* FINISH ALPHA CHANGE 3061 -- verify generated full-history nodes use one TiB -- */
			/* FINISH ALPHA CHANGE 1739 -- test generated full-node checkpoint snapshot source -- */
		} else {
			/* START ALPHA CHANGE 1740 -- test generated recent-node segmented snapshot source -- */
			require.Contains(t, envContent, "INNBC_HISTORY_MODE=recent")
			/* START ALPHA CHANGE 1770 -- retain segmented geometry in recent-node environments -- */
			require.Contains(t, envContent, "INNBC_HISTORY_RETENTION_BLOCKS=43200")
			require.Contains(t, envContent, "INNBC_HISTORY_SEGMENT_BLOCKS=1800")
			/* FINISH ALPHA CHANGE 1770 -- retain segmented geometry in recent-node environments -- */
			require.Contains(
				t,
				envContent,
				fmt.Sprintf(
					"INNBC_SNAPSHOT_SOURCE_DIR=%s",
					filepath.Join(
						/* START ALPHA CHANGE 1980 -- expect the independent segmented snapshot root -- */
						snapshotRoot,
						"segmented",
						/* FINISH ALPHA CHANGE 1980 -- expect the independent segmented snapshot root -- */
					),
				),
			)
			require.Contains(t, envContent, "INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED=false")
			require.NotContains(t, envContent, "INNBC_SNAPSHOT_INTERVAL_BLOCKS")
			require.NotContains(t, envContent, "INNBC_SNAPSHOT_DIR=")
			require.NotContains(t, envContent, "INNBC_SNAPSHOT_PRODUCER_ENABLED")
			/* START ALPHA CHANGE 1796 -- verify one recent-node MDBX map size -- */
			require.Contains(t, envContent, "INNBC_MDBX_MAP_SIZE_GB=128")
			/* FINISH ALPHA CHANGE 1796 -- verify one recent-node MDBX map size -- */
			/* FINISH ALPHA CHANGE 1740 -- test generated recent-node segmented snapshot source -- */
		}
		/* FINISH ALPHA CHANGE 1065 -- test generated node storage and snapshot defaults -- */
		/* START ALPHA CHANGE 1622 -- verify independently prepared identity directories -- */
		require.DirExists(t, filepath.Join(baseDir, nodeName, "keys", "validator-ecdsa"))
		require.DirExists(t, filepath.Join(baseDir, nodeName, "keys", "validator-bls"))
		require.DirExists(t, filepath.Join(baseDir, nodeName, "keys", "libp2p"))
		/* FINISH ALPHA CHANGE 1622 -- verify independently prepared identity directories -- */
	}

	require.NoFileExists(t, filepath.Join(configDir, "node7.env"))
	/* START ALPHA CHANGE 1970 -- keep snapshot creation outside node preparation -- */
	require.NoDirExists(t, snapshotRoot)
	/* FINISH ALPHA CHANGE 1970 -- keep snapshot creation outside node preparation -- */
}

/* START ALPHA CHANGE 3357 -- verify complete allocation planning and protected destinations -- */

func nodeAllocationConfig(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	original := *createConfig
	t.Cleanup(func() {
		*createConfig = original
	})

	*createConfig = createParams{
		baseDir:      filepath.Join(root, "nodes"),
		configDir:    filepath.Join(root, "config"),
		snapshotRoot: filepath.Join(root, "snapshots"),
		chainPath:    filepath.Join(root, "genesis.json"),
		mode:         privateLocalMode,
	}

	return root
}

func nodeAllocationParents(t *testing.T) {
	t.Helper()

	require.NoError(t, os.MkdirAll(createConfig.baseDir, 0750))
	require.NoError(t, os.MkdirAll(createConfig.configDir, 0755))
}

func nodeAllocationFiles(t *testing.T, count int) map[string][]byte {
	t.Helper()

	files := make(map[string][]byte)
	for number := 1; number <= count; number++ {
		name := fmt.Sprintf("node%d", number)
		dir := filepath.Join(createConfig.baseDir, name)
		for _, path := range []string{
			datadir.ValidatorKey(dir),
			datadir.ValidatorBLSKey(dir),
			datadir.LibP2PKey(dir),
			filepath.Join(createConfig.configDir, name+".env"),
		} {
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			files[path] = content
		}
	}

	return files
}

func nodeAllocationAssertFiles(t *testing.T, files map[string][]byte) {
	t.Helper()

	for path, before := range files {
		after, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, before, after, "changed existing file %s", path)
	}
}

func TestNodeAllocation_PlanningIsReadOnly(t *testing.T) {
	root := nodeAllocationConfig(t)

	numbers, err := planNodeCreation(
		createConfig.baseDir, createConfig.configDir, 3,
	)
	require.NoError(t, err)
	require.Equal(t, []int{1, 2, 3}, numbers)

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestNodeAllocation_ReservedPathsAndGaps(t *testing.T) {
	root := nodeAllocationConfig(t)
	nodeAllocationParents(t)

	require.NoError(t, os.Mkdir(filepath.Join(createConfig.baseDir, "node1"), 0700))
	require.NoError(t, os.WriteFile(
		filepath.Join(createConfig.baseDir, "node2"), []byte("identity reservation"), 0600,
	))
	require.NoError(t, os.Symlink(
		filepath.Join(root, "missing-identity-target"),
		filepath.Join(createConfig.baseDir, "node3"),
	))
	require.NoError(t, os.WriteFile(
		filepath.Join(createConfig.configDir, "node4.env"), []byte("env reservation"), 0600,
	))
	require.NoError(t, os.Mkdir(
		filepath.Join(createConfig.configDir, "node5.env"), 0700,
	))
	require.NoError(t, os.Symlink(
		filepath.Join(root, "missing-env-target"),
		filepath.Join(createConfig.configDir, "node6.env"),
	))

	target := filepath.Join(root, "existing-env-target")
	require.NoError(t, os.WriteFile(target, []byte("preserve target"), 0600))
	require.NoError(t, os.Symlink(
		target, filepath.Join(createConfig.configDir, "node7.env"),
	))

	numbers, err := planNodeCreation(
		createConfig.baseDir, createConfig.configDir, 2,
	)
	require.NoError(t, err)
	require.Equal(t, []int{8, 9}, numbers)
	require.NoDirExists(t, filepath.Join(createConfig.baseDir, "node8"))
	require.NoFileExists(t, filepath.Join(createConfig.configDir, "node8.env"))

	require.NoError(t, os.Mkdir(filepath.Join(createConfig.baseDir, "node9"), 0700))
	numbers, err = planNodeCreation(
		createConfig.baseDir, createConfig.configDir, 2,
	)
	require.NoError(t, err)
	require.Equal(t, []int{8, 10}, numbers)

	// These direct calls simulate destinations appearing after planning.
	require.ErrorIs(t, createPlannedNode(2), os.ErrExist)
	require.ErrorIs(t, createPlannedNode(3), os.ErrExist)
	require.NoFileExists(t, filepath.Join(createConfig.configDir, "node2.env"))
	require.NoFileExists(t, filepath.Join(createConfig.configDir, "node3.env"))

	content, err := os.ReadFile(target)
	require.NoError(t, err)
	require.Equal(t, "preserve target", string(content))
	require.NoFileExists(t, filepath.Join(root, "missing-env-target"))
	require.NoDirExists(t, filepath.Join(root, "missing-identity-target"))
}

func TestNodeAllocation_InspectionErrors(t *testing.T) {
	for _, name := range []string{"base", "config"} {
		t.Run(name, func(t *testing.T) {
			root := nodeAllocationConfig(t)
			blocked := filepath.Join(root, "regular-file")
			require.NoError(t, os.WriteFile(blocked, []byte("preserve"), 0600))

			if name == "base" {
				createConfig.baseDir = blocked
			} else {
				createConfig.configDir = blocked
			}

			numbers, err := planNodeCreation(
				createConfig.baseDir, createConfig.configDir, 1,
			)
			require.Error(t, err)
			require.Nil(t, numbers)

			var pathError *os.PathError
			require.ErrorAs(t, err, &pathError)
			require.Equal(t, "lstat", pathError.Op)

			content, err := os.ReadFile(blocked)
			require.NoError(t, err)
			require.Equal(t, "preserve", string(content))
		})
	}
}

func TestNodeAllocation_InvalidInputsBeforeWrites(t *testing.T) {
	for _, count := range []string{
		"0", "-1", "11", "invalid",
		"9999999999999999999999999999999999999999",
	} {
		t.Run("count="+count, func(t *testing.T) {
			root := nodeAllocationConfig(t)

			require.Error(t, runCreateCommand(nil, []string{count}))
			require.Error(t, runPrepareCommand(nil, []string{count}))

			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}

	for _, field := range []string{"mode", "snapshot", "base", "config"} {
		t.Run(field, func(t *testing.T) {
			root := nodeAllocationConfig(t)
			switch field {
			case "mode":
				createConfig.mode = "unsupported"
			case "snapshot":
				createConfig.snapshotRoot = " \t "
			case "base":
				createConfig.baseDir = ""
			case "config":
				createConfig.configDir = ""
			}

			require.Error(t, runCreateCommand(nil, []string{"1"}))
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}

	for _, count := range []int{0, -1, maxCreateCount + 1} {
		numbers, err := planNodeCreation("unused-base", "unused-config", count)
		require.Error(t, err)
		require.Nil(t, numbers)
	}
}

func TestNodeAllocation_SuccessiveBatches(t *testing.T) {
	for _, mode := range []string{privateLocalMode, externalValidatorMode} {
		t.Run(mode, func(t *testing.T) {
			nodeAllocationConfig(t)
			createConfig.mode = mode

			require.NoError(t, runCreateCommand(nil, []string{"10"}))
			firstBatch := nodeAllocationFiles(t, 10)

			require.NoError(t, runCreateCommand(nil, []string{"10"}))
			nodeAllocationAssertFiles(t, firstBatch)

			for number := 1; number <= 20; number++ {
				name := fmt.Sprintf("node%d", number)
				dir := filepath.Join(createConfig.baseDir, name)
				require.NoError(t, validateNodeIdentityFolder(dir))

				content, err := os.ReadFile(
					filepath.Join(createConfig.configDir, name+".env"),
				)
				require.NoError(t, err)
				require.Contains(t, string(content),
					fmt.Sprintf("INNBC_JSONRPC_ADDR=127.0.0.1:%d\n", 8544+number))
				require.Contains(t, string(content),
					fmt.Sprintf("INNBC_GRPC_ADDR=127.0.0.1:%d\n", 9631+number))

				host := "127.0.0.1"
				if mode == externalValidatorMode {
					host = "0.0.0.0"
				}
				require.Contains(t, string(content),
					fmt.Sprintf("INNBC_LIBP2P_ADDR=%s:%d\n", host, 30300+number))

				if number == 1 {
					require.Contains(t, string(content), "INNBC_HISTORY_MODE=full\n")
				} else {
					require.Contains(t, string(content), "INNBC_HISTORY_MODE=recent\n")
				}
			}

			require.NoError(t, runInfoCommand(nil, []string{"node11"}))
			require.NoError(t, runInfoCommand(nil, []string{"node20"}))
			require.NoDirExists(t, createConfig.snapshotRoot)
			require.NoFileExists(t, createConfig.chainPath)
		})
	}
}

func TestNodeAllocation_Capacity(t *testing.T) {
	nodeAllocationConfig(t)
	require.NoError(t, os.MkdirAll(createConfig.baseDir, 0750))

	for number := 1; number < maxNodeNumber; number++ {
		require.NoError(t, os.Mkdir(
			filepath.Join(createConfig.baseDir, fmt.Sprintf("node%d", number)), 0700,
		))
	}

	numbers, err := planNodeCreation(
		createConfig.baseDir, createConfig.configDir, 2,
	)
	require.ErrorContains(t, err, "only 1 available slot remains")
	require.Nil(t, numbers)

	require.ErrorContains(t,
		runCreateCommand(nil, []string{"2"}), "only 1 available slot remains")
	require.NoDirExists(t, createConfig.configDir)
	require.NoDirExists(t, filepath.Join(createConfig.baseDir, "node100"))

	numbers, err = planNodeCreation(
		createConfig.baseDir, createConfig.configDir, 1,
	)
	require.NoError(t, err)
	require.Equal(t, []int{100}, numbers)

	require.NoError(t, runCreateCommand(nil, []string{"1"}))
	require.NoError(t, validateNodeIdentityFolder(
		filepath.Join(createConfig.baseDir, "node100"),
	))
	require.NoError(t, runInfoCommand(nil, []string{"node100"}))

	envPath := filepath.Join(createConfig.configDir, "node100.env")
	before, err := os.ReadFile(envPath)
	require.NoError(t, err)
	require.Contains(t, string(before), "INNBC_JSONRPC_ADDR=127.0.0.1:8644\n")
	require.Contains(t, string(before), "INNBC_GRPC_ADDR=127.0.0.1:9731\n")
	require.Contains(t, string(before), "INNBC_LIBP2P_ADDR=127.0.0.1:30400\n")

	require.ErrorContains(t,
		runCreateCommand(nil, []string{"1"}), "only 0 available slots remain")
	require.Error(t, runInfoCommand(nil, []string{"node101"}))
	require.NoDirExists(t, filepath.Join(createConfig.baseDir, "node101"))

	after, err := os.ReadFile(envPath)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestNodeAllocation_CheckedPortsAndNames(t *testing.T) {
	used := make(map[int]string)

	for number := 1; number <= maxNodeNumber; number++ {
		jsonRPC, grpc, libp2p, err := nodePorts(number)
		require.NoError(t, err)
		require.Equal(t, 8544+number, jsonRPC)
		require.Equal(t, 9631+number, grpc)
		require.Equal(t, 30300+number, libp2p)

		for _, port := range []int{jsonRPC, grpc, libp2p} {
			require.Greater(t, port, 0)
			require.LessOrEqual(t, port, 65535)
			require.NotContains(t, used, port)
			used[port] = fmt.Sprintf("node%d", number)
		}
	}

	for _, number := range []int{-1, 0, maxNodeNumber + 1, int(^uint(0) >> 1)} {
		_, _, _, err := nodePorts(number)
		require.Error(t, err)
	}

	for _, number := range []int{1, 10, 11, 100} {
		actual, err := parseNodeNumber(fmt.Sprintf("node%d", number))
		require.NoError(t, err)
		require.Equal(t, number, actual)
	}

	for _, name := range []string{
		"node0", "node-1", "node101", "other",
		"node999999999999999999999999999999999999",
	} {
		_, err := parseNodeNumber(name)
		require.Error(t, err)
	}

	root := nodeAllocationConfig(t)
	missing := filepath.Join(createConfig.baseDir, "node101")
	require.Error(t, createPlannedNode(101))
	require.Error(t, writeNodeEnvFile(createConfig.configDir, "node101", 101, privateLocalMode))
	require.Error(t, printPreparedNode("node101", missing, 101))
	require.Error(t, printNodeIdentity("CREATED", "node101", missing, 101))
	/* START ALPHA CHANGE 3363 -- exercise checked numbering through the read-only identity loader -- */
	_, infoErr := readNodePublicInfo(missing, 101)
	require.Error(t, infoErr)
	/* FINISH ALPHA CHANGE 3363 -- exercise checked numbering through the read-only identity loader -- */

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestNodeAllocation_ExistingIdentityPreserved(t *testing.T) {
	nodeAllocationConfig(t)
	nodeAllocationParents(t)

	require.NoError(t, createPlannedNode(1))
	before := nodeAllocationFiles(t, 1)

	require.ErrorIs(t, createPlannedNode(1), os.ErrExist)
	nodeAllocationAssertFiles(t, before)
}

func TestNodeAllocation_EnvironmentReservation(t *testing.T) {
	for _, kind := range []string{"file", "directory", "symlink", "dangling"} {
		t.Run(kind, func(t *testing.T) {
			root := nodeAllocationConfig(t)
			nodeAllocationParents(t)

			envPath := filepath.Join(createConfig.configDir, "node1.env")
			target := filepath.Join(root, "target")
			sentinel := []byte("preserve existing environment\n")

			switch kind {
			case "file":
				require.NoError(t, os.WriteFile(envPath, sentinel, 0600))
			case "directory":
				require.NoError(t, os.Mkdir(envPath, 0700))
				require.NoError(t, os.WriteFile(
					filepath.Join(envPath, "sentinel"), sentinel, 0600,
				))
			case "symlink":
				require.NoError(t, os.WriteFile(target, sentinel, 0600))
				require.NoError(t, os.Symlink(target, envPath))
			case "dangling":
				require.NoError(t, os.Symlink(target, envPath))
			}

			// Simulate an env destination appearing after the batch was planned.
			require.ErrorIs(t, createPlannedNode(1), os.ErrExist)
			require.DirExists(t, filepath.Join(createConfig.baseDir, "node1"))
			require.NoDirExists(t, filepath.Join(createConfig.baseDir, "node1", "keys"))

			switch kind {
			case "file":
				actual, err := os.ReadFile(envPath)
				require.NoError(t, err)
				require.Equal(t, sentinel, actual)
			case "directory":
				actual, err := os.ReadFile(filepath.Join(envPath, "sentinel"))
				require.NoError(t, err)
				require.Equal(t, sentinel, actual)
			default:
				actualTarget, err := os.Readlink(envPath)
				require.NoError(t, err)
				require.Equal(t, target, actualTarget)
				if kind == "symlink" {
					actual, err := os.ReadFile(target)
					require.NoError(t, err)
					require.Equal(t, sentinel, actual)
				} else {
					require.NoFileExists(t, target)
				}
			}
		})
	}
}

func TestNodeAllocation_ConcurrentCreators(t *testing.T) {
	nodeAllocationConfig(t)
	nodeAllocationParents(t)

	first, err := planNodeCreation(
		createConfig.baseDir, createConfig.configDir, 1,
	)
	require.NoError(t, err)
	second, err := planNodeCreation(
		createConfig.baseDir, createConfig.configDir, 1,
	)
	require.NoError(t, err)
	require.Equal(t, first, second)

	start := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	workers.Add(2)

	for _, number := range []int{first[0], second[0]} {
		go func(number int) {
			defer workers.Done()
			<-start
			results <- createPlannedNode(number)
		}(number)
	}

	close(start)
	workers.Wait()
	close(results)

	successes := 0
	failures := 0
	for err := range results {
		if err == nil {
			successes++
		} else {
			failures++
			require.ErrorIs(t, err, os.ErrExist)
		}
	}

	require.Equal(t, 1, successes)
	require.Equal(t, 1, failures)
	require.NoError(t, validateNodeIdentityFolder(
		filepath.Join(createConfig.baseDir, "node1"),
	))

	content, err := os.ReadFile(filepath.Join(createConfig.configDir, "node1.env"))
	require.NoError(t, err)
	require.Contains(t, string(content), "INNBC_JSONRPC_ADDR=127.0.0.1:8545\n")
}

/* FINISH ALPHA CHANGE 3357 -- verify complete allocation planning and protected destinations -- */

/* START ALPHA CHANGE 3363 -- verify inspection without repair writes or private-key output -- */

type nodeInfoFileSnapshot struct {
	Mode     os.FileMode
	Modified int64
	Content  string
}

func nodeInfoSnapshot(t *testing.T, root string) map[string]nodeInfoFileSnapshot {
	t.Helper()

	result := make(map[string]nodeInfoFileSnapshot)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		entry := nodeInfoFileSnapshot{
			Mode:     info.Mode(),
			Modified: info.ModTime().UnixNano(),
		}
		if info.Mode().IsRegular() {
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			entry.Content = string(content)
		} else if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			entry.Content = target
		}
		result[path] = entry
		return nil
	})
	require.NoError(t, err)

	return result
}

func captureNodeInfo(t *testing.T, args ...string) (string, error) {
	t.Helper()

	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	original := os.Stdout
	defer func() {
		os.Stdout = original
		_ = writer.Close()
		_ = reader.Close()
	}()

	done := make(chan []byte, 1)
	go func() {
		content, _ := io.ReadAll(reader)
		done <- content
	}()

	os.Stdout = writer
	runErr := runInfoCommand(nil, args)
	require.NoError(t, writer.Close())
	os.Stdout = original

	return string(<-done), runErr
}

func TestNodeInfo_ReadOnlyPublicIdentity(t *testing.T) {
	root := nodeAllocationConfig(t)
	require.NoError(t, runCreateCommand(nil, []string{"1"}))

	files := nodeAllocationFiles(t, 1)
	envPath := filepath.Join(createConfig.configDir, "node1.env")
	env, err := os.ReadFile(envPath)
	require.NoError(t, err)
	privateKey := files[datadir.ValidatorKey(filepath.Join(createConfig.baseDir, "node1"))]
	env = append(env, []byte("\nINNBC_PRIVATE_KEY="+string(privateKey)+"\n")...)
	require.NoError(t, os.WriteFile(envPath, env, 0600))

	// Restore directory permissions only during test cleanup.
	t.Cleanup(func() {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err == nil && info.IsDir() {
				_ = os.Chmod(path, 0700)
			}
			return nil
		})
	})
	require.NoError(t, filepath.Walk(root,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return os.Chmod(path, 0500)
			}
			return os.Chmod(path, 0400)
		},
	))

	before := nodeInfoSnapshot(t, root)
	output, err := captureNodeInfo(t, "node1")
	require.NoError(t, err)
	require.Contains(t, output, "Identity status|valid")
	require.Contains(t, output, "Environment status|valid")
	require.Contains(t, output, "Operator address / registry msg.sender|")
	require.Contains(t, output, "BLS proof / registerValidator blsProof|")
	require.NotContains(t, output, "INNBC_PRIVATE_KEY")

	for path, content := range files {
		if filepath.Ext(path) != ".env" {
			require.NotContains(t, output, string(content))
		}
	}
	require.Equal(t, before, nodeInfoSnapshot(t, root))
}

func TestNodeInfo_AllGapsAndContinuedInspection(t *testing.T) {
	root := nodeAllocationConfig(t)
	require.NoError(t, runCreateCommand(nil, []string{"10"}))

	// Construct an intentional unoccupied gap between existing nodes.
	require.NoError(t, os.RemoveAll(filepath.Join(createConfig.baseDir, "node4")))
	require.NoError(t, os.Remove(filepath.Join(createConfig.configDir, "node4.env")))

	before := nodeInfoSnapshot(t, root)
	output, err := captureNodeInfo(t, "all")
	require.NoError(t, err)
	require.Contains(t, output, "WARNING: NODE 4 IDENTITY MISSING")
	require.Contains(t, output, "Name|node10")
	require.NotContains(t, output, "Name|node11")
	require.Equal(t, before, nodeInfoSnapshot(t, root))

	// An incomplete occupied slot must not hide later healthy nodes.
	require.NoError(t, os.Remove(
		datadir.ValidatorKey(filepath.Join(createConfig.baseDir, "node2")),
	))
	before = nodeInfoSnapshot(t, root)
	output, err = captureNodeInfo(t, "all")
	require.Error(t, err)
	require.Contains(t, output, "ERROR: NODE 2")
	require.Contains(t, output, "Name|node10")
	require.Equal(t, before, nodeInfoSnapshot(t, root))
	require.NoFileExists(t,
		datadir.ValidatorKey(filepath.Join(createConfig.baseDir, "node2")))
}

func TestNodeInfo_CorruptKeysDoNotLeak(t *testing.T) {
	cases := []struct {
		name string
		path func(string) string
		data string
	}{
		{"ecdsa_encoding", datadir.ValidatorKey, "PRIVATE_ECDSA_CANARY"},
		{"ecdsa_zero", datadir.ValidatorKey, strings.Repeat("0", 64)},
		{"ecdsa_range", datadir.ValidatorKey, strings.Repeat("f", 64)},
		{"bls_encoding", datadir.ValidatorBLSKey, "PRIVATE_BLS_CANARY"},
		{"bls_zero", datadir.ValidatorBLSKey, "0"},
		{"bls_negative", datadir.ValidatorBLSKey, "-1"},
		{"bls_range", datadir.ValidatorBLSKey, strings.Repeat("9", 100)},
		{"network_encoding", datadir.LibP2PKey, "PRIVATE_NETWORK_CANARY"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			root := nodeAllocationConfig(t)
			require.NoError(t, runCreateCommand(nil, []string{"1"}))
			path := test.path(filepath.Join(createConfig.baseDir, "node1"))
			/* START ALPHA CHANGE 3366 -- prepare corrupt key fixtures and restore protected permissions before inspection -- */
			keyInfo, err := os.Stat(path)
			require.NoError(t, err)
			require.NoError(t, os.Chmod(path, 0600))
			require.NoError(t, os.WriteFile(path, []byte(test.data), 0600))
			require.NoError(t, os.Chmod(path, keyInfo.Mode().Perm()))
			/* FINISH ALPHA CHANGE 3366 -- prepare corrupt key fixtures and restore protected permissions before inspection -- */
			before := nodeInfoSnapshot(t, root)

			output, err := captureNodeInfo(t, "node1")
			require.Error(t, err)
			require.Contains(t, output, "Identity status|invalid")
			require.Contains(t, output, "Environment status|valid")
			if strings.Contains(test.data, "CANARY") {
				require.NotContains(t, output+err.Error(), test.data)
			}
			require.Equal(t, before, nodeInfoSnapshot(t, root))
		})
	}
}

func TestNodeInfo_EnvironmentFailuresAndRedaction(t *testing.T) {
	for _, scenario := range []string{
		"missing", "empty", "malformed", "duplicate", "wrong_directory",
		"invalid_address", "directory", "symlink", "unknown_secret", "url_credentials",
	} {
		t.Run(scenario, func(t *testing.T) {
			root := nodeAllocationConfig(t)
			require.NoError(t, runCreateCommand(nil, []string{"1"}))
			envPath := filepath.Join(createConfig.configDir, "node1.env")
			original, err := os.ReadFile(envPath)
			require.NoError(t, err)
			content := string(original)

			switch scenario {
			case "missing":
				require.NoError(t, os.Remove(envPath))
			case "empty":
				require.NoError(t, os.WriteFile(envPath, nil, 0600))
			case "malformed":
				require.NoError(t, os.WriteFile(envPath,
					[]byte("PRIVATE_ENV_CANARY\n"), 0600))
			case "duplicate":
				content += "\nINNBC_CHAIN=PRIVATE_ENV_CANARY\n"
			case "wrong_directory":
				content = strings.Replace(content,
					"INNBC_DATA_DIR="+filepath.Join(createConfig.baseDir, "node1"),
					"INNBC_DATA_DIR=/wrong/PRIVATE_ENV_CANARY", 1)
			case "invalid_address":
				content = strings.Replace(content,
					"INNBC_GRPC_ADDR=127.0.0.1:9632",
					"INNBC_GRPC_ADDR=PRIVATE_ENV_CANARY", 1)
			case "directory":
				require.NoError(t, os.Remove(envPath))
				require.NoError(t, os.Mkdir(envPath, 0700))
			case "symlink":
				target := filepath.Join(root, "private-target")
				require.NoError(t, os.WriteFile(target,
					[]byte("PRIVATE_ENV_CANARY"), 0600))
				require.NoError(t, os.Remove(envPath))
				require.NoError(t, os.Symlink(target, envPath))
			case "unknown_secret":
				content += "\nINNBC_ACCESS_TOKEN=PRIVATE_ENV_CANARY\n"
			case "url_credentials":
				content = strings.Replace(content,
					"http://127.0.0.1:8080/api/v1/peers/exchange",
					"https://alice:PRIVATE_ENV_CANARY@example.org/api?token=PRIVATE_ENV_CANARY#PRIVATE_ENV_CANARY", 1)
			}

			switch scenario {
			case "duplicate", "wrong_directory", "invalid_address",
				"unknown_secret", "url_credentials":
				require.NoError(t, os.WriteFile(envPath, []byte(content), 0600))
			}

			before := nodeInfoSnapshot(t, root)
			output, runErr := captureNodeInfo(t, "node1")
			require.Contains(t, output, "Identity status|valid")
			require.Contains(t, output, "Operator address / registry msg.sender|")
			require.NotContains(t, output, "PRIVATE_ENV_CANARY")
			require.NotContains(t, output, "INNBC_ACCESS_TOKEN")

			if scenario == "unknown_secret" || scenario == "url_credentials" {
				require.NoError(t, runErr)
				require.Contains(t, output, "Environment status|valid")
				if scenario == "url_credentials" {
					require.Contains(t, output, "https://example.org/api")
					require.NotContains(t, output, "alice")
				}
			} else {
				require.Error(t, runErr)
				if scenario == "missing" {
					require.Contains(t, output, "WARNING: NODE 1 ENVIRONMENT MISSING")
				} else {
					require.Contains(t, output, "Environment status|invalid")
				}
			}
			require.Equal(t, before, nodeInfoSnapshot(t, root))
		})
	}
}

func TestNodeInfo_EmptyAndEnvironmentOnlyInstallations(t *testing.T) {
	root := nodeAllocationConfig(t)

	output, err := captureNodeInfo(t, "all")
	require.NoError(t, err)
	require.Contains(t, output, "No node identities or environments found.")
	require.Empty(t, nodeInfoSnapshot(t, root)[root].Content)

	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)

	nodeAllocationParents(t)
	require.NoError(t, writeNodeEnvFile(
		createConfig.configDir, "node3", 3, privateLocalMode,
	))
	before := nodeInfoSnapshot(t, root)

	output, err = captureNodeInfo(t, "all")
	require.Error(t, err)
	require.Contains(t, output, "WARNING: NODE 3 IDENTITY MISSING")
	require.Contains(t, output, "Environment status|valid")
	require.NotContains(t, output, "Name|node4")
	require.NoDirExists(t, filepath.Join(createConfig.baseDir, "node3"))
	require.Equal(t, before, nodeInfoSnapshot(t, root))
}

/* FINISH ALPHA CHANGE 3363 -- verify inspection without repair writes or private-key output -- */
