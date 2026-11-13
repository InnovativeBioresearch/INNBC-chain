/* START ALPHA CHANGE 3237 -- add an isolated fixture for current INNBC CLI workflows -- */
package root

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	"github.com/stretchr/testify/require"
)

const (
	cwChildEnvironment = "INNBC_CURRENT_WORKFLOW_CHILD"

	cwOperatorField = "Operator address / registry msg.sender"
	cwBLSField      = "BLS public key / registerValidator blsKey"
	cwProofField    = "BLS proof / registerValidator blsProof"
	cwPeerField     = "Libp2p peer ID / registerValidator libp2pPeerID"
)

// Only child processes execute the real CLI, whose handlers may call os.Exit.
func TestCurrentWorkflow_Process(t *testing.T) {
	if os.Getenv(cwChildEnvironment) != "1" {
		return
	}

	separator := slices.Index(os.Args, "--")
	if separator < 0 {
		fmt.Fprintln(os.Stderr, "missing workflow subprocess separator")
		os.Exit(2)
	}

	cmd := NewRootCommand()
	cmd.baseCmd.SetArgs(os.Args[separator+1:])
	cmd.Execute()

	// Keep testing-framework output out of command responses.
	os.Exit(0)
}

func cwCommand(t *testing.T, dir string, args ...string) *exec.Cmd {
	t.Helper()

	executable, err := os.Executable()
	require.NoError(t, err)

	childArgs := append(
		[]string{"-test.run=^TestCurrentWorkflow_Process$", "--"},
		args...,
	)

	cmd := exec.Command(executable, childArgs...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), cwChildEnvironment+"=1")

	return cmd
}

func cwCLI(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	/* START ALPHA CHANGE 3240 -- retain explicit subprocess timeout handling without an invalid Cancel callback -- */
	cmd := cwCommand(t, dir, args...)
	/* FINISH ALPHA CHANGE 3240 -- retain explicit subprocess timeout handling without an invalid Cancel callback -- */

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	require.NoError(t, cmd.Start())

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			return stdout.String(), fmt.Errorf("%w: %s", err, stderr.String())
		}
		return stdout.String(), nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		<-done
		t.Fatalf("workflow command timed out: %v", args)
		return "", ctx.Err()
	}
}

func cwRequireCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()

	output, err := cwCLI(t, dir, args...)
	require.NoError(t, err, "command %v\nstdout:\n%s", args, output)

	return output
}

type cwProcess struct {
	cmd      *exec.Cmd
	done     chan struct{}
	err      error
	logPath  string
	graceful bool
	stopped  bool
}

type cwNode struct {
	dir       string
	env       map[string]string
	info      map[string]string
	keys      map[string][]byte
	rpc       string
	grpc      string
	multiaddr string
	args      []string
	process   *cwProcess
}

type cwFixture struct {
	dir       string
	base      string
	config    string
	snapshots string
	chain     string
	genesis   []byte
	nodes     []*cwNode
	processes []*cwProcess
}

func cwPaths(t *testing.T) *cwFixture {
	t.Helper()

	dir := t.TempDir()
	return &cwFixture{
		dir:       dir,
		base:      filepath.Join(dir, "nodes"),
		config:    filepath.Join(dir, "config"),
		snapshots: filepath.Join(dir, "snapshots"),
		chain:     filepath.Join(dir, "genesis.json"),
	}
}

func (f *cwFixture) createArgs(count int, mode string) []string {
	return []string{
		"node", "create", strconv.Itoa(count),
		"--base-dir", f.base,
		"--config-dir", f.config,
		"--snapshot-root", f.snapshots,
		"--chain", f.chain,
		"--mode", mode,
	}
}

func cwProvision(t *testing.T, count int, mode string) *cwFixture {
	t.Helper()

	f := cwPaths(t)
	cwRequireCLI(t, f.dir, f.createArgs(count, mode)...)

	for number := 1; number <= count; number++ {
		name := fmt.Sprintf("node%d", number)
		dir := filepath.Join(f.base, name)

		/* START ALPHA CHANGE 3364 -- inspect the environment belonging to the workflow identity -- */
		output := cwRequireCLI(
			t, f.dir,
			"node", "info", name,
			"--base-dir", f.base,
			"--config-dir", f.config,
		)
		/* FINISH ALPHA CHANGE 3364 -- inspect the environment belonging to the workflow identity -- */

		info := make(map[string]string)
		for _, line := range strings.Split(output, "\n") {
			key, value, ok := strings.Cut(line, "|")
			if ok {
				info[strings.TrimSpace(key)] = strings.TrimSpace(value)
			}
		}
		for _, field := range []string{
			cwOperatorField, cwBLSField, cwProofField, cwPeerField,
		} {
			require.NotEmpty(t, info[field], "missing node info field %q", field)
		}

		f.nodes = append(f.nodes, &cwNode{
			dir:  dir,
			env:  cwReadEnv(t, filepath.Join(f.config, name+".env")),
			info: info,
			keys: cwReadKeys(t, dir),
		})
	}

	return f
}

func cwReadEnv(t *testing.T, path string) map[string]string {
	t.Helper()

	content, err := os.ReadFile(path)
	require.NoError(t, err)

	values := make(map[string]string)
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		require.True(t, ok, "invalid environment line %q", line)
		_, duplicate := values[key]
		require.False(t, duplicate, "duplicate environment setting %q", key)
		values[key] = value
	}

	return values
}

func cwReadKeys(t *testing.T, dir string) map[string][]byte {
	t.Helper()

	result := make(map[string][]byte)
	for _, path := range []string{
		datadir.ValidatorKey(dir),
		datadir.ValidatorBLSKey(dir),
		datadir.LibP2PKey(dir),
	} {
		content, err := os.ReadFile(path)
		require.NoError(t, err)
		require.NotEmpty(t, content, "empty identity file %s", path)
		result[path] = content
	}

	return result
}

func cwAssertKeysUnchanged(t *testing.T, node *cwNode) {
	t.Helper()

	for path, expected := range node.keys {
		actual, err := os.ReadFile(path)
		require.NoError(t, err)
		// Avoid printing private key material when reporting a failure.
		require.True(t, bytes.Equal(expected, actual), "identity changed: %s", path)
	}
}

func (f *cwFixture) start(
	t *testing.T,
	graceful bool,
	args ...string,
) *cwProcess {
	t.Helper()

	logFile, err := os.CreateTemp(f.dir, "workflow-process-*.log")
	require.NoError(t, err)

	cmd := cwCommand(t, f.dir, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		require.NoError(t, err)
	}

	process := &cwProcess{
		cmd:      cmd,
		done:     make(chan struct{}),
		logPath:  logFile.Name(),
		graceful: graceful,
	}

	go func() {
		process.err = cmd.Wait()
		_ = logFile.Close()
		close(process.done)
	}()

	f.processes = append(f.processes, process)
	t.Cleanup(func() {
		cwStop(t, process)
	})

	return process
}

func cwLogTail(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	if len(content) > 8192 {
		content = content[len(content)-8192:]
	}
	return string(content)
}

func cwStop(t *testing.T, process *cwProcess) {
	t.Helper()

	if process == nil || process.stopped {
		return
	}
	process.stopped = true

	select {
	case <-process.done:
		t.Errorf("workflow process exited unexpectedly: %v\n%s",
			process.err, cwLogTail(process.logPath))
		return
	default:
	}

	if err := process.cmd.Process.Signal(os.Interrupt); err != nil {
		t.Errorf("failed to interrupt workflow process: %v", err)
		_ = process.cmd.Process.Kill()
	}

	select {
	case <-process.done:
		if process.graceful && process.err != nil {
			t.Errorf("node failed during graceful shutdown: %v\n%s",
				process.err, cwLogTail(process.logPath))
		}
	case <-time.After(30 * time.Second):
		_ = process.cmd.Process.Kill()
		t.Errorf("workflow process required forced termination\n%s",
			cwLogTail(process.logPath))
		select {
		case <-process.done:
		case <-time.After(5 * time.Second):
			t.Error("workflow process did not exit after forced termination")
		}
	}
}

// Reserve ports together, releasing each immediately before its listener starts.
func cwReserveAddress(t *testing.T) (string, func()) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	release := func() {
		_ = listener.Close()
	}
	t.Cleanup(release)

	return listener.Addr().String(), release
}

func (f *cwFixture) wait(
	t *testing.T,
	description string,
	check func() (bool, error),
) {
	t.Helper()

	deadline := time.NewTimer(90 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	var lastError error
	for {
		for _, process := range f.processes {
			if process.stopped {
				continue
			}
			select {
			case <-process.done:
				t.Fatalf("%s: process exited: %v\n%s",
					description, process.err, cwLogTail(process.logPath))
			default:
			}
		}

		ready, err := check()
		if ready && err == nil {
			return
		}
		lastError = err

		select {
		case <-ticker.C:
		case <-deadline.C:
			for _, process := range f.processes {
				t.Log(cwLogTail(process.logPath))
			}
			t.Fatalf("timed out waiting for %s; last error: %v",
				description, lastError)
		case <-t.Context().Done():
			t.Fatalf("test cancelled while waiting for %s", description)
		}
	}
}

type cwBlock struct {
	Hash         string            `json:"hash"`
	ParentHash   string            `json:"parentHash"`
	Number       string            `json:"number"`
	BaseFee      string            `json:"baseFeePerGas"`
	Miner        string            `json:"miner"`
	Timestamp    string            `json:"timestamp"`
	Transactions []json.RawMessage `json:"transactions"`
}

type cwReceipt struct {
	Hash              string `json:"transactionHash"`
	BlockHash         string `json:"blockHash"`
	BlockNumber       string `json:"blockNumber"`
	Status            string `json:"status"`
	GasUsed           string `json:"gasUsed"`
	EffectiveGasPrice string `json:"effectiveGasPrice"`
	ContractAddress   string `json:"contractAddress"`
}

func cwDecodeRPC(body io.Reader, result any) error {
	var response struct {
		Version string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	content, err := io.ReadAll(io.LimitReader(body, 4*1024*1024+1))
	if err != nil {
		return err
	}
	if len(content) > 4*1024*1024 {
		return fmt.Errorf("workflow RPC response exceeds four MiB")
	}
	if err := json.Unmarshal(content, &response); err != nil {
		return err
	}
	if response.Version != "2.0" || response.ID != 1 {
		return fmt.Errorf("unexpected RPC envelope")
	}
	if response.Error != nil {
		return fmt.Errorf("RPC error %d: %s",
			response.Error.Code, response.Error.Message)
	}
	if len(response.Result) == 0 {
		return fmt.Errorf("RPC response has no result")
	}

	return json.Unmarshal(response.Result, result)
}

/* FINISH ALPHA CHANGE 3237 -- add an isolated fixture for current INNBC CLI workflows -- */
