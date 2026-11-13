package node

import (
	/* START ALPHA CHANGE 3360 -- support read-only node identity and environment inspection -- */
	encodinghex "encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/url"
	"os"

	/* FINISH ALPHA CHANGE 3360 -- support read-only node identity and environment inspection -- */
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	/* START ALPHA CHANGE 3360 -- import public identity derivation and existing output formatting -- */
	"github.com/innovativebioresearch/innbc-chain/bls"
	"github.com/innovativebioresearch/innbc-chain/command"
	chaincrypto "github.com/innovativebioresearch/innbc-chain/crypto"
	"github.com/innovativebioresearch/innbc-chain/network"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	bn256 "github.com/umbracle/go-eth-bn256"

	/* FINISH ALPHA CHANGE 3360 -- import public identity derivation and existing output formatting -- */

	/* START ALPHA CHANGE 3060 -- generate full-node storage from the shared MDBX default -- */
	serverconfig "github.com/innovativebioresearch/innbc-chain/command/server/config"
	/* FINISH ALPHA CHANGE 3060 -- generate full-node storage from the shared MDBX default -- */

	/* START ALPHA CHANGE 2023 -- generate public validator possession proofs for registration -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	/* FINISH ALPHA CHANGE 2023 -- generate public validator possession proofs for registration -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	/* START ALPHA CHANGE 2461 -- generate fee-history settings from shared defaults -- */
	"github.com/innovativebioresearch/innbc-chain/gasprice"
	/* FINISH ALPHA CHANGE 2461 -- generate fee-history settings from shared defaults -- */
	/* START ALPHA CHANGE 2496 -- generate event-log settings from the shared default -- */
	"github.com/innovativebioresearch/innbc-chain/jsonrpc"
	/* FINISH ALPHA CHANGE 2496 -- generate event-log settings from the shared default -- */
	"github.com/innovativebioresearch/innbc-chain/secrets"

	/* START ALPHA CHANGE 1617 -- use canonical node data paths -- */
	"github.com/innovativebioresearch/innbc-chain/helper/datadir"
	/* FINISH ALPHA CHANGE 1617 -- use canonical node data paths -- */
	"github.com/innovativebioresearch/innbc-chain/helper/hex"
	secretshelper "github.com/innovativebioresearch/innbc-chain/secrets/helper"
	"github.com/spf13/cobra"
)

const (
	defaultConfigDir    = "/etc/innbc"
	defaultBaseDir      = "/var/lib/innbc"
	defaultSnapshotRoot = "/var/lib/innbc/snapshots"

	defaultChainPath = "/etc/innbc/genesis.json"

	privateLocalMode      = "private-local"
	externalValidatorMode = "external-validator"

	/* START ALPHA CHANGE 3354 -- separate the creation batch limit from supported node numbering -- */
	maxCreateCount = 10
	maxNodeNumber  = 100
	/* FINISH ALPHA CHANGE 3354 -- separate the creation batch limit from supported node numbering -- */
)

type createParams struct {
	configDir    string
	baseDir      string
	snapshotRoot string
	chainPath    string
	mode         string
}

var createConfig = &createParams{
	configDir:    defaultConfigDir,
	baseDir:      defaultBaseDir,
	snapshotRoot: defaultSnapshotRoot,
	chainPath:    defaultChainPath,
	mode:         privateLocalMode,
}

func GetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "node",
		Short: "Manages installed INNBC validator node services",
	}

	cmd.AddCommand(
		getCreateCommand(),
		/* START ALPHA CHANGE 2115 -- register offline trie rebuild operation -- */
		getTrieRebuildCommand(),
		/* FINISH ALPHA CHANGE 2115 -- register offline trie rebuild operation -- */
		/* START ALPHA CHANGE 765 -- replace import workflow with empty node preparation command -- */
		getPrepareCommand(),
		/* FINISH ALPHA CHANGE 765 -- replace import workflow with empty node preparation command -- */
		/* START ALPHA CHANGE 771 -- add public validator registration info command -- */
		getInfoCommand(),
		/* FINISH ALPHA CHANGE 771 -- add public validator registration info command -- */
		getServiceCommand("start", "Starts one or all INNBC validator node services and enables boot startup"),
		getServiceCommand("stop", "Stops one or all INNBC validator node services"),
		getServiceCommand("restart", "Restarts one or all INNBC validator node services"),
		getServiceCommand("status", "Shows one or all INNBC validator node service statuses"),
		getServiceCommand("disable", "Stops one or all INNBC validator node services and disables boot startup"),
	)

	return cmd
}

func getCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create [count]",
		Short: "Creates INNBC validator node identity folders and service env files",
		/* START ALPHA CHANGE 3381 -- document bounded creation and read-only follow-up inspection in CLI help -- */
		Long: `Creates one to ten new identities per invocation using available numbers
within node1-node100. Existing identity or environment paths reserve their numbers.

The complete allocation is checked before writing. A later execution failure can
leave completed nodes and an incomplete current slot; there is no batch rollback.

After provisioning, use node info nodeN or node info all with the same --base-dir
and --config-dir. Inspection validates stored keys and selected environment
structure read-only. It does not print private keys or repair incomplete slots.`,
		/* FINISH ALPHA CHANGE 3381 -- document bounded creation and read-only follow-up inspection in CLI help -- */
		Args: cobra.MaximumNArgs(1),
		RunE: runCreateCommand,
	}

	cmd.Flags().StringVar(
		&createConfig.configDir,
		"config-dir",
		defaultConfigDir,
		"the directory containing INNBC service env files",
	)

	cmd.Flags().StringVar(
		&createConfig.baseDir,
		"base-dir",
		defaultBaseDir,
		"the base directory containing INNBC validator node data directories",
	)

	/* START ALPHA CHANGE 1966 -- configure snapshot storage independently from node data -- */
	cmd.Flags().StringVar(
		&createConfig.snapshotRoot,
		"snapshot-root",
		defaultSnapshotRoot,
		"the shared snapshot root written into generated INNBC service env files",
	)
	/* FINISH ALPHA CHANGE 1966 -- configure snapshot storage independently from node data -- */

	/* START ALPHA CHANGE 817 -- allow generated node env files to target local or installed genesis paths -- */
	cmd.Flags().StringVar(
		&createConfig.chainPath,
		"chain",
		defaultChainPath,
		"the genesis file path written into generated INNBC service env files",
	)
	/* FINISH ALPHA CHANGE 817 -- allow generated node env files to target local or installed genesis paths -- */

	cmd.Flags().StringVar(
		&createConfig.mode,
		"mode",
		privateLocalMode,
		"node profile mode: private-local or external-validator",
	)

	return cmd
}

/* START ALPHA CHANGE 766 -- prepare empty validator identity folders for manual key placement -- */
func getPrepareCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prepare [count]",
		Short: "Creates empty INNBC validator node folders and service env files",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runPrepareCommand,
	}

	cmd.Flags().StringVar(
		&createConfig.configDir,
		"config-dir",
		defaultConfigDir,
		"the directory containing INNBC service env files",
	)

	cmd.Flags().StringVar(
		&createConfig.baseDir,
		"base-dir",
		defaultBaseDir,
		"the base directory containing INNBC validator node data directories",
	)

	/* START ALPHA CHANGE 1967 -- configure prepared-node snapshot storage independently -- */
	cmd.Flags().StringVar(
		&createConfig.snapshotRoot,
		"snapshot-root",
		defaultSnapshotRoot,
		"the shared snapshot root written into generated INNBC service env files",
	)
	/* FINISH ALPHA CHANGE 1967 -- configure prepared-node snapshot storage independently -- */

	/* START ALPHA CHANGE 818 -- allow prepared node env files to target local or installed genesis paths -- */
	cmd.Flags().StringVar(
		&createConfig.chainPath,
		"chain",
		defaultChainPath,
		"the genesis file path written into generated INNBC service env files",
	)
	/* FINISH ALPHA CHANGE 818 -- allow prepared node env files to target local or installed genesis paths -- */

	cmd.Flags().StringVar(
		&createConfig.mode,
		"mode",
		privateLocalMode,
		"node profile mode: private-local or external-validator",
	)

	return cmd
}

func runPrepareCommand(_ *cobra.Command, args []string) error {
	count := 1

	if len(args) == 1 {
		parsedCount, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid node count: %w", err)
		}

		count = parsedCount
	}

	if count < 1 || count > maxCreateCount {
		return fmt.Errorf("node count must be between 1 and %d", maxCreateCount)
	}

	if createConfig.mode != privateLocalMode && createConfig.mode != externalValidatorMode {
		return fmt.Errorf("unsupported node mode %q", createConfig.mode)
	}

	if err := os.MkdirAll(createConfig.configDir, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(createConfig.baseDir, 0750); err != nil {
		return err
	}

	/* START ALPHA CHANGE 944 -- prepare deterministic node slots instead of next free slots -- */
	for nodeNumber := 1; nodeNumber <= count; nodeNumber++ {
		nodeName := fmt.Sprintf("node%d", nodeNumber)
		nodeDir := filepath.Join(createConfig.baseDir, nodeName)
		/* FINISH ALPHA CHANGE 944 -- prepare deterministic node slots instead of next free slots -- */

		if err := prepareNodeIdentityFolders(nodeDir); err != nil {
			return fmt.Errorf("failed to prepare %s folders: %w", nodeName, err)
		}

		if err := writeNodeEnvFile(createConfig.configDir, nodeName, nodeNumber, createConfig.mode); err != nil {
			return fmt.Errorf("failed to write %s env file: %w", nodeName, err)
		}

		/* START ALPHA CHANGE 3356 -- propagate checked preparation output errors without changing target selection -- */
		if err := printPreparedNode(nodeName, nodeDir, nodeNumber); err != nil {
			return err
		}
		/* FINISH ALPHA CHANGE 3356 -- propagate checked preparation output errors without changing target selection -- */
	}

	return nil
}

func prepareNodeIdentityFolders(nodeDir string) error {
	/* START ALPHA CHANGE 1618 -- prepare isolated identity directories only -- */
	requiredDirs := []string{
		nodeDir,
		datadir.ValidatorECDSAKeys(nodeDir),
		datadir.ValidatorBLSKeys(nodeDir),
		datadir.LibP2PKeys(nodeDir),
	}

	for _, requiredDir := range requiredDirs {
		if err := os.MkdirAll(requiredDir, 0700); err != nil {
			return err
		}
	}
	/* FINISH ALPHA CHANGE 1618 -- prepare isolated identity directories only -- */

	return nil
}

/* START ALPHA CHANGE 3356 -- share checked port assignments in preparation output -- */
func printPreparedNode(nodeName, nodeDir string, nodeNumber int) error {
	jsonRPCPort, grpcPort, libp2pPort, err := nodePorts(nodeNumber)
	if err != nil {
		return err
	}

	fmt.Printf("\n[INNBC NODE PREPARED]\n")
	fmt.Printf("Name|%s\n", nodeName)
	fmt.Printf("Data directory|%s\n", nodeDir)
	fmt.Printf("Env file|%s\n", filepath.Join(createConfig.configDir, nodeName+".env"))
	/* START ALPHA CHANGE 1619 -- report canonical identity key destinations -- */
	fmt.Printf("Expected validator key|%s\n", datadir.ValidatorKey(nodeDir))
	fmt.Printf("Expected validator BLS key|%s\n", datadir.ValidatorBLSKey(nodeDir))
	fmt.Printf("Expected libp2p key|%s\n", datadir.LibP2PKey(nodeDir))
	/* FINISH ALPHA CHANGE 1619 -- report canonical identity key destinations -- */
	fmt.Printf("JSON-RPC port|%d\n", jsonRPCPort)
	fmt.Printf("GRPC port|%d\n", grpcPort)
	fmt.Printf("Libp2p port|%d\n", libp2pPort)

	return nil
}

/* FINISH ALPHA CHANGE 3356 -- share checked port assignments in preparation output -- */

func validateNodeIdentityFolder(nodeDir string) error {
	requiredDirs := []struct {
		label string
		path  string
	}{
		{"node directory", nodeDir},
		/* START ALPHA CHANGE 1620 -- validate each independent identity directory -- */
		{"validator ECDSA key directory", datadir.ValidatorECDSAKeys(nodeDir)},
		{"validator BLS key directory", datadir.ValidatorBLSKeys(nodeDir)},
		{"libp2p key directory", datadir.LibP2PKeys(nodeDir)},
		/* FINISH ALPHA CHANGE 1620 -- validate each independent identity directory -- */
	}

	for _, requiredDir := range requiredDirs {
		info, err := os.Stat(requiredDir.path)
		if err != nil {
			return fmt.Errorf("missing %s: %s", requiredDir.label, requiredDir.path)
		}

		if !info.IsDir() {
			return fmt.Errorf("%s is not a directory: %s", requiredDir.label, requiredDir.path)
		}
	}

	requiredFiles := []struct {
		label string
		path  string
	}{
		/* START ALPHA CHANGE 1621 -- validate canonical identity key files -- */
		{"validator key", datadir.ValidatorKey(nodeDir)},
		{"validator BLS key", datadir.ValidatorBLSKey(nodeDir)},
		{"libp2p key", datadir.LibP2PKey(nodeDir)},
		/* FINISH ALPHA CHANGE 1621 -- validate canonical identity key files -- */
	}

	for _, requiredFile := range requiredFiles {
		info, err := os.Stat(requiredFile.path)
		if err != nil {
			return fmt.Errorf("missing %s: %s", requiredFile.label, requiredFile.path)
		}

		if info.IsDir() {
			return fmt.Errorf("%s is a directory, expected key file: %s", requiredFile.label, requiredFile.path)
		}
	}

	return nil
}

/* FINISH ALPHA CHANGE 766 -- prepare empty validator identity folders for manual key placement -- */

/* START ALPHA CHANGE 772 -- print public validator registration information from node keys -- */
/* START ALPHA CHANGE 3360 -- inspect one node or discovered slots and report failure after complete output -- */
func getInfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info <node-name|all>",
		Short: "Inspects node identities and public environment settings without modifying files",
		/* START ALPHA CHANGE 3381 -- describe read-only inspection discovery and validation limits in CLI help -- */
		Long: `Inspects one supported node, node1-node100, or all discovered slots.

The all selection runs from node1 through the highest supported number having
an identity or environment path. Completely absent interior slots produce
warnings. Missing trailing nodes beyond that number are not inferred.

Stored keys are validated locally and only public identity values are reported.
Environment files are inspected independently without executing them. Unknown
settings are omitted; displayed peer-directory URLs omit user information,
query parameters and fragments.

Inspection does not initialize storage, modify files, repair identities or
establish complete runtime readiness. Invalid occupied slots cause failure
after reporting. An empty discovery result succeeds.

Use the same --base-dir and --config-dir as provisioning. Add --json for a
structured report on stdout; terminal failures are reported on stderr.`,
		/* FINISH ALPHA CHANGE 3381 -- describe read-only inspection discovery and validation limits in CLI help -- */
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := runInfoCommand(cmd, args); err != nil {
				outputter := command.InitializeOutputter(cmd)
				outputter.SetError(err)
				outputter.WriteOutput()
				os.Exit(1)
			}
		},
	}

	cmd.Flags().StringVar(
		&createConfig.baseDir,
		"base-dir",
		defaultBaseDir,
		"the base directory containing INNBC validator node data directories",
	)
	cmd.Flags().StringVar(
		&createConfig.configDir,
		"config-dir",
		defaultConfigDir,
		"the directory containing INNBC service env files",
	)

	return cmd
}

func runInfoCommand(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(createConfig.baseDir) == "" {
		return errors.New("node base directory must not be empty")
	}
	if strings.TrimSpace(createConfig.configDir) == "" {
		return errors.New("node config directory must not be empty")
	}

	report := &nodeInfoReport{Nodes: make([]nodeInfoEntry, 0)}

	if args[0] == "all" {
		highest := 0
		for number := 1; number <= maxNodeNumber; number++ {
			name := fmt.Sprintf("node%d", number)
			identityPresent, err := nodePathOccupied(
				filepath.Join(createConfig.baseDir, name),
			)
			if err != nil {
				return err
			}
			envPresent, err := nodePathOccupied(
				filepath.Join(createConfig.configDir, name+".env"),
			)
			if err != nil {
				return err
			}

			if identityPresent || envPresent {
				highest = number
			}
		}

		for number := 1; number <= highest; number++ {
			report.Nodes = append(report.Nodes, inspectNodeInfo(
				fmt.Sprintf("node%d", number), number, true,
			))
		}
	} else {
		number, err := parseNodeNumber(args[0])
		if err != nil {
			return err
		}
		report.Nodes = append(report.Nodes, inspectNodeInfo(args[0], number, false))
	}

	if cmd == nil {
		// Direct callers used by unit tests retain the public text format.
		fmt.Print(report.GetOutput())
	} else {
		outputter := command.InitializeOutputter(cmd)
		outputter.SetCommandResult(report)
		outputter.WriteOutput()
	}

	failures := 0
	for _, entry := range report.Nodes {
		if len(entry.Errors) > 0 {
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("node inspection failed for %d node(s)", failures)
	}

	return nil
}

/* FINISH ALPHA CHANGE 3360 -- inspect one node or discovered slots and report failure after complete output -- */

/* START ALPHA CHANGE 3361 -- derive validated public identities without initializing local storage -- */
func readNodePublicInfo(nodeDir string, nodeNumber int) (*nodePublicInfo, error) {
	_, _, libp2pPort, err := nodePorts(nodeNumber)
	if err != nil {
		return nil, err
	}
	if err := validateNodeIdentityFolder(nodeDir); err != nil {
		return nil, err
	}

	encodedECDSA, err := readNodeInfoFile(datadir.ValidatorKey(nodeDir), 4096)
	if err != nil {
		return nil, err
	}
	rawECDSA, err := encodinghex.DecodeString(string(encodedECDSA))
	if err != nil || len(rawECDSA) != 32 {
		return nil, errors.New("invalid validator ECDSA key encoding or length")
	}
	ecdsaScalar := new(big.Int).SetBytes(rawECDSA)
	if ecdsaScalar.Sign() <= 0 || ecdsaScalar.Cmp(chaincrypto.S256.Params().N) >= 0 {
		return nil, errors.New("invalid validator ECDSA key scalar")
	}
	ecdsaKey, err := chaincrypto.ParseECDSAPrivateKey(rawECDSA)
	if err != nil {
		return nil, errors.New("invalid validator ECDSA key")
	}
	address := chaincrypto.PubKeyToAddress(&ecdsaKey.PublicKey)

	encodedBLS, err := readNodeInfoFile(datadir.ValidatorBLSKey(nodeDir), 4096)
	if err != nil {
		return nil, err
	}

	// Match the runtime's existing decimal/prefixed-number/hex decoding order,
	// but reject invalid scalars before performing curve operations.
	blsScalar := new(big.Int)
	if err := blsScalar.UnmarshalText(encodedBLS); err != nil {
		blsScalar, err = hex.DecodeHexToBig(string(encodedBLS))
		if err != nil {
			return nil, errors.New("invalid validator BLS key encoding")
		}
	}
	if blsScalar.Sign() <= 0 || blsScalar.Cmp(bn256.Order) >= 0 {
		return nil, errors.New("invalid validator BLS key scalar")
	}
	blsKey, err := bls.UnmarshalPrivateKey(encodedBLS)
	if err != nil {
		return nil, errors.New("invalid validator BLS key")
	}

	encodedNetwork, err := readNodeInfoFile(datadir.LibP2PKey(nodeDir), 4096)
	if err != nil {
		return nil, err
	}
	networkKey, err := network.ParseLibp2pKey(encodedNetwork)
	if err != nil {
		return nil, errors.New("invalid libp2p key encoding")
	}

	if networkKey.Type() == libp2pcrypto.Secp256k1 {
		raw, err := networkKey.Raw()
		if err != nil || len(raw) != 32 {
			return nil, errors.New("invalid libp2p secp256k1 key")
		}
		scalar := new(big.Int).SetBytes(raw)
		if scalar.Sign() <= 0 || scalar.Cmp(chaincrypto.S256.Params().N) >= 0 {
			return nil, errors.New("invalid libp2p secp256k1 key scalar")
		}
	}

	challenge := []byte("INNBC_NODE_INFO_KEY_VALIDATION")
	signature, err := networkKey.Sign(challenge)
	if err != nil {
		return nil, errors.New("invalid libp2p signing key")
	}
	valid, err := networkKey.GetPublic().Verify(challenge, signature)
	if err != nil || !valid {
		return nil, errors.New("libp2p private and public key validation failed")
	}
	nodeID, err := peer.IDFromPrivateKey(networkKey)
	if err != nil {
		return nil, errors.New("cannot derive libp2p peer ID")
	}

	publicKeyBytes := blsKey.PublicKey().Marshal()
	proof, err := signer.MakeINNBCBLSPoP(blsKey, address)
	if err != nil {
		return nil, errors.New("invalid validator BLS signing key")
	}
	proofBytes, err := proof.Marshal()
	if err != nil {
		return nil, errors.New("cannot encode validator BLS proof")
	}
	if _, err := signer.VerifyINNBCBLSPoP(publicKeyBytes, address, proofBytes); err != nil {
		return nil, errors.New("validator BLS proof validation failed")
	}

	// Only public values leave this function. Decoder errors are deliberately
	// not wrapped because some decoders include supplied key text in errors.
	return &nodePublicInfo{
		OperatorAddress:   address.String(),
		BLSPublicKey:      hex.EncodeToHex(publicKeyBytes),
		BLSProof:          hex.EncodeToHex(proofBytes),
		PeerID:            nodeID.String(),
		DefaultLibp2pPort: libp2pPort,
	}, nil
}

/* FINISH ALPHA CHANGE 3361 -- derive validated public identities without initializing local storage -- */

/* FINISH ALPHA CHANGE 772 -- print public validator registration information from node keys -- */

/* START ALPHA CHANGE 3362 -- report independent identity and public environment inspection results -- */

type nodePublicInfo struct {
	OperatorAddress   string `json:"operator_address"`
	BLSPublicKey      string `json:"bls_public_key"`
	BLSProof          string `json:"bls_proof"`
	PeerID            string `json:"peer_id"`
	DefaultLibp2pPort int    `json:"default_libp2p_port"`
}

type nodeInfoEntry struct {
	Name              string            `json:"name"`
	Number            int               `json:"number"`
	DataDirectory     string            `json:"data_directory"`
	EnvironmentFile   string            `json:"environment_file"`
	IdentityStatus    string            `json:"identity_status"`
	EnvironmentStatus string            `json:"environment_status"`
	Public            *nodePublicInfo   `json:"public,omitempty"`
	Environment       map[string]string `json:"environment,omitempty"`
	Warnings          []string          `json:"warnings,omitempty"`
	Errors            []string          `json:"errors,omitempty"`
}

type nodeInfoReport struct {
	Nodes []nodeInfoEntry `json:"nodes"`
}

func (r *nodeInfoReport) GetOutput() string {
	if len(r.Nodes) == 0 {
		return "No node identities or environments found.\n"
	}

	var output strings.Builder
	for _, entry := range r.Nodes {
		fmt.Fprintf(&output, "\n[INNBC NODE INFO]\nName|%s\nData directory|%s\n",
			entry.Name, entry.DataDirectory)
		fmt.Fprintf(&output, "Identity status|%s\n", entry.IdentityStatus)

		if public := entry.Public; public != nil {
			fmt.Fprintf(&output, "Operator address / registry msg.sender|%s\n",
				public.OperatorAddress)
			fmt.Fprintf(&output, "BLS public key / registerValidator blsKey|%s\n",
				public.BLSPublicKey)
			fmt.Fprintf(&output, "BLS proof / registerValidator blsProof|%s\n",
				public.BLSProof)
			fmt.Fprintf(&output, "Libp2p peer ID / registerValidator libp2pPeerID|%s\n",
				public.PeerID)
			fmt.Fprintf(&output, "Private-local multiaddr|/ip4/127.0.0.1/tcp/%d/p2p/%s\n",
				public.DefaultLibp2pPort, public.PeerID)
			fmt.Fprintf(&output, "External-validator multiaddr template|/ip4/<server-ip>/tcp/%d/p2p/%s\n",
				public.DefaultLibp2pPort, public.PeerID)
			output.WriteString("Registry function|registerValidator(rewardAddress, blsKey, blsProof, libp2pPeerID, stakeAmount)\n")
			output.WriteString("Registry rewardAddress|manual input, not derived from node keys\n")
			output.WriteString("Registry stakeAmount|manual input, must be greater than or equal to minimumStake\n")
		}

		fmt.Fprintf(&output, "Environment file|%s\nEnvironment status|%s\n",
			entry.EnvironmentFile, entry.EnvironmentStatus)
		for _, name := range nodeInfoEnvNames {
			if value, ok := entry.Environment[name]; ok {
				// Quote stored values so control characters cannot alter output.
				fmt.Fprintf(&output, "%s|%s\n", name, strconv.Quote(value))
			}
		}
		for _, warning := range entry.Warnings {
			fmt.Fprintln(&output, warning)
		}
		for _, message := range entry.Errors {
			fmt.Fprintf(&output, "ERROR: NODE %d %s\n", entry.Number, message)
		}
	}

	return output.String()
}

func inspectNodeInfo(name string, number int, allowGap bool) nodeInfoEntry {
	entry := nodeInfoEntry{
		Name:              name,
		Number:            number,
		DataDirectory:     filepath.Join(createConfig.baseDir, name),
		EnvironmentFile:   filepath.Join(createConfig.configDir, name+".env"),
		IdentityStatus:    "missing",
		EnvironmentStatus: "missing",
	}

	identityPresent, identityErr := nodePathOccupied(entry.DataDirectory)
	envPresent, envErr := nodePathOccupied(entry.EnvironmentFile)

	if allowGap && identityErr == nil && envErr == nil &&
		!identityPresent && !envPresent {
		entry.Warnings = append(entry.Warnings,
			fmt.Sprintf("WARNING: NODE %d IDENTITY MISSING", number))
		return entry
	}

	switch {
	case identityErr != nil:
		entry.IdentityStatus = "invalid"
		entry.Errors = append(entry.Errors, identityErr.Error())
	case !identityPresent:
		entry.Warnings = append(entry.Warnings,
			fmt.Sprintf("WARNING: NODE %d IDENTITY MISSING", number))
		entry.Errors = append(entry.Errors, "identity is missing")
	default:
		public, err := readNodePublicInfo(entry.DataDirectory, number)
		if err != nil {
			entry.IdentityStatus = "invalid"
			entry.Errors = append(entry.Errors, err.Error())
		} else {
			entry.IdentityStatus = "valid"
			entry.Public = public
		}
	}

	// Inspect the environment even when identity validation failed.
	switch {
	case envErr != nil:
		entry.EnvironmentStatus = "invalid"
		entry.Errors = append(entry.Errors, envErr.Error())
	case !envPresent:
		entry.Warnings = append(entry.Warnings,
			fmt.Sprintf("WARNING: NODE %d ENVIRONMENT MISSING", number))
		entry.Errors = append(entry.Errors, "environment is missing")
	default:
		values, err := readNodeInfoEnvironment(entry.EnvironmentFile, entry.DataDirectory)
		if err != nil {
			entry.EnvironmentStatus = "invalid"
			entry.Errors = append(entry.Errors, err.Error())
		} else {
			entry.EnvironmentStatus = "valid"
			entry.Environment = values
		}
	}

	return entry
}

func readNodeInfoFile(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot inspect file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("inspection requires a regular file: %s", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read file %s: %w", path, err)
	}
	defer file.Close()

	info, err = file.Stat()
	if err != nil {
		return nil, fmt.Errorf("cannot inspect opened file %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("inspection requires a regular file: %s", path)
	}

	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, fmt.Errorf("cannot read file %s: %w", path, err)
	}
	if int64(len(content)) > limit {
		return nil, fmt.Errorf("inspection file exceeds size limit: %s", path)
	}

	return content, nil
}

// Only these recognized public settings can appear in inspection output.
// Unknown settings, including any manually added private-key/token variables,
// are never copied into the report.
var nodeInfoEnvNames = []string{
	"INNBC_CHAIN", "INNBC_DATA_DIR",
	"INNBC_JSONRPC_ADDR", "INNBC_GRPC_ADDR", "INNBC_LIBP2P_ADDR",
	"INNBC_NAT_ADDR", "INNBC_DNS_ADDR",
	"INNBC_PEER_DIRECTORY_ENABLED", "INNBC_PEER_DIRECTORY_URL",
	"INNBC_HISTORY_MODE", "INNBC_HISTORY_RETENTION_BLOCKS",
	"INNBC_HISTORY_SEGMENT_BLOCKS", "INNBC_MDBX_MAP_SIZE_GB",
	/* START ALPHA CHANGE 3476 -- expose the public page-size environment setting in node inspection -- */
	"INNBC_MDBX_PAGE_SIZE_BYTES",
	/* FINISH ALPHA CHANGE 3476 -- expose the public page-size environment setting in node inspection -- */
	"INNBC_SNAPSHOT_SOURCE_DIR", "INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED",
	"INNBC_JSONRPC_BLOCK_RANGE_LIMIT",
	"INNBC_JSONRPC_FEE_HISTORY_BLOCK_LIMIT",
	"INNBC_JSONRPC_FEE_HISTORY_PERCENTILE_LIMIT",
	"INNBC_JSONRPC_FEE_HISTORY_RESULT_LIMIT",
	"INNBC_JSONRPC_REQUEST_RATE", "INNBC_JSONRPC_REQUEST_BURST",
	"INNBC_JSONRPC_CALL_RATE", "INNBC_JSONRPC_CALL_BURST",
	"INNBC_JSONRPC_MAX_CONCURRENT", "INNBC_JSONRPC_MAX_EXPENSIVE",
	"INNBC_JSONRPC_MAX_BODY_BYTES", "INNBC_JSONRPC_MAX_WEBSOCKETS",
	"INNBC_JSONRPC_MAX_FILTERS",
}

func readNodeInfoEnvironment(path, nodeDir string) (map[string]string, error) {
	content, err := readNodeInfoFile(path, 64*1024)
	if err != nil {
		return nil, err
	}

	allowed := make(map[string]bool, len(nodeInfoEnvNames))
	for _, name := range nodeInfoEnvNames {
		allowed[name] = true
	}
	values := make(map[string]string)

	for index, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		name, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("invalid environment assignment at line %d", index+1)
		}
		name = strings.TrimSpace(name)
		if !allowed[name] {
			continue
		}
		if _, exists := values[name]; exists {
			return nil, fmt.Errorf("duplicate public environment setting at line %d", index+1)
		}

		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, "\"") {
			value, err = strconv.Unquote(value)
			if err != nil {
				return nil, fmt.Errorf("invalid quoted environment value at line %d", index+1)
			}
		} else if strings.HasPrefix(value, "'") {
			if len(value) < 2 || !strings.HasSuffix(value, "'") {
				return nil, fmt.Errorf("invalid quoted environment value at line %d", index+1)
			}
			value = value[1 : len(value)-1]
			if strings.Contains(value, "'") {
				return nil, fmt.Errorf("unsupported quoted environment value at line %d", index+1)
			}
		}
		if strings.ContainsAny(value, "\x00\r\n") {
			return nil, fmt.Errorf("invalid environment value at line %d", index+1)
		}

		values[name] = value
	}

	for _, name := range []string{
		"INNBC_CHAIN", "INNBC_DATA_DIR",
		"INNBC_JSONRPC_ADDR", "INNBC_GRPC_ADDR", "INNBC_LIBP2P_ADDR",
	} {
		if strings.TrimSpace(values[name]) == "" {
			return nil, fmt.Errorf("required public environment setting is missing: %s", name)
		}
	}

	actualDir, err := filepath.Abs(values["INNBC_DATA_DIR"])
	if err != nil {
		return nil, errors.New("invalid environment data directory")
	}
	expectedDir, err := filepath.Abs(nodeDir)
	if err != nil || actualDir != expectedDir {
		return nil, errors.New("environment data directory does not match selected node")
	}

	for _, name := range []string{
		"INNBC_JSONRPC_ADDR", "INNBC_GRPC_ADDR", "INNBC_LIBP2P_ADDR",
	} {
		_, port, err := net.SplitHostPort(values[name])
		if err != nil {
			return nil, fmt.Errorf("invalid public environment address: %s", name)
		}
		number, err := strconv.Atoi(port)
		if err != nil || number < 0 || number > 65535 {
			return nil, fmt.Errorf("invalid public environment port: %s", name)
		}
	}

	if mode, exists := values["INNBC_HISTORY_MODE"]; exists &&
		mode != "full" && mode != "recent" {
		return nil, errors.New("invalid public environment history mode")
	}

	if endpoint, exists := values["INNBC_PEER_DIRECTORY_URL"]; exists {
		parsed, err := url.Parse(endpoint)
		if err != nil || parsed.Host == "" ||
			(parsed.Scheme != "http" && parsed.Scheme != "https") {
			return nil, errors.New("invalid public peer-directory URL")
		}
		// Credentials and query/fragment parameters are not public output.
		parsed.User = nil
		parsed.RawQuery = ""
		parsed.ForceQuery = false
		parsed.Fragment = ""
		parsed.RawFragment = ""
		values["INNBC_PEER_DIRECTORY_URL"] = parsed.String()
	}

	return values, nil
}

/* FINISH ALPHA CHANGE 3362 -- report independent identity and public environment inspection results -- */

/* START ALPHA CHANGE 3355 -- plan complete identity batches before writes and reserve each destination exclusively -- */
func runCreateCommand(_ *cobra.Command, args []string) error {
	count := 1

	if len(args) == 1 {
		parsedCount, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid node count: %w", err)
		}

		count = parsedCount
	}

	if count < 1 || count > maxCreateCount {
		return fmt.Errorf("node count must be between 1 and %d", maxCreateCount)
	}

	if createConfig.mode != privateLocalMode && createConfig.mode != externalValidatorMode {
		return fmt.Errorf("unsupported node mode %q", createConfig.mode)
	}

	if strings.TrimSpace(createConfig.baseDir) == "" {
		return errors.New("node base directory must not be empty")
	}

	if strings.TrimSpace(createConfig.configDir) == "" {
		return errors.New("node config directory must not be empty")
	}

	if strings.TrimSpace(createConfig.snapshotRoot) == "" {
		return errors.New("snapshot root must not be empty")
	}

	nodeNumbers, err := planNodeCreation(
		createConfig.baseDir,
		createConfig.configDir,
		count,
	)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(createConfig.configDir, 0755); err != nil {
		return err
	}

	if err := os.MkdirAll(createConfig.baseDir, 0750); err != nil {
		return err
	}

	for _, nodeNumber := range nodeNumbers {
		if err := createPlannedNode(nodeNumber); err != nil {
			return err
		}
	}

	return nil
}

func createPlannedNode(nodeNumber int) error {
	if _, _, _, err := nodePorts(nodeNumber); err != nil {
		return err
	}

	nodeName := fmt.Sprintf("node%d", nodeNumber)
	nodeDir := filepath.Join(createConfig.baseDir, nodeName)

	// Mkdir deliberately refuses existing directories, files and symlinks.
	// A successful preflight does not grant ownership of a destination.
	if err := os.Mkdir(nodeDir, 0700); err != nil {
		return fmt.Errorf("failed to reserve %s directory: %w", nodeName, err)
	}

	// Reserve and write the environment before generating private keys.
	// Failure can leave our new directory, but never overwrites another env.
	if err := writeNodeEnvFile(
		createConfig.configDir,
		nodeName,
		nodeNumber,
		createConfig.mode,
	); err != nil {
		return fmt.Errorf("failed to write %s env file: %w", nodeName, err)
	}

	if err := createNodeIdentity(nodeDir); err != nil {
		return fmt.Errorf("failed to create %s identity: %w", nodeName, err)
	}

	return printNodeIdentity("CREATED", nodeName, nodeDir, nodeNumber)
}

/* FINISH ALPHA CHANGE 3355 -- plan complete identity batches before writes and reserve each destination exclusively -- */

func createNodeIdentity(nodeDir string) error {
	manager, err := secretshelper.SetupLocalSecretsManager(nodeDir)
	if err != nil {
		return err
	}

	if !manager.HasSecret(secrets.NetworkKey) {
		if _, err := secretshelper.InitNetworkingPrivateKey(manager); err != nil {
			return err
		}
	}

	if !manager.HasSecret(secrets.ValidatorKey) && !manager.HasSecret(secrets.ValidatorBLSKey) {
		account, err := wallet.GenerateAccount()
		if err != nil {
			return err
		}

		if err := account.Save(manager); err != nil {
			return err
		}
	}

	return nil
}

func writeNodeEnvFile(configDir, nodeName string, nodeNumber int, mode string) error {
	/* START ALPHA CHANGE 3355 -- validate generated ports before opening an environment destination -- */
	jsonRPCPort, grpcPort, libp2pPort, err := nodePorts(nodeNumber)
	if err != nil {
		return err
	}

	envPath := filepath.Join(configDir, nodeName+".env")
	libp2pAddr := fmt.Sprintf("127.0.0.1:%d", libp2pPort)
	/* FINISH ALPHA CHANGE 3355 -- validate generated ports before opening an environment destination -- */
	/* START ALPHA CHANGE 1205 -- generate optional advertised NAT and DNS addresses -- */
	natAddr := ""
	dnsAddr := ""
	/* FINISH ALPHA CHANGE 1205 -- generate optional advertised NAT and DNS addresses -- */
	peerDirectoryURL := "http://127.0.0.1:8080/api/v1/peers/exchange"
	/* START ALPHA CHANGE 2496 -- omit historical query overrides from recent-node environments -- */
	blockRangeEnv := ""
	feeHistoryEnv := ""
	/* FINISH ALPHA CHANGE 2496 -- omit historical query overrides from recent-node environments -- */
	/* START ALPHA CHANGE 1768 -- generate role-specific history and snapshot settings -- */
	historyMode := "recent"
	historyGeometry := `# START ALPHA CHANGE 1579 -- generate one-day segmented recent-history defaults --
INNBC_HISTORY_RETENTION_BLOCKS=43200
INNBC_HISTORY_SEGMENT_BLOCKS=1800
# FINISH ALPHA CHANGE 1579 -- generate one-day segmented recent-history defaults --
`
	/* START ALPHA CHANGE 1792 -- generate one node-wide MDBX map size -- */
	mdbxMapSizeGB := uint64(128)
	/* FINISH ALPHA CHANGE 1792 -- generate one node-wide MDBX map size -- */
	/* START ALPHA CHANGE 1968 -- generate snapshot sources from the shared snapshot root -- */
	snapshotRoot := strings.TrimSpace(createConfig.snapshotRoot)
	if snapshotRoot == "" {
		return errors.New("snapshot root must not be empty")
	}

	snapshotSourceDir := filepath.Join(
		snapshotRoot,
		"segmented",
	)

	if nodeNumber == 1 {
		historyMode = "full"
		historyGeometry = ""

		/* START ALPHA CHANGE 3060 -- give generated full-history nodes the shared MDBX allowance -- */
		mdbxMapSizeGB = serverconfig.DefaultMDBXMapSizeGB
		/* FINISH ALPHA CHANGE 3060 -- give generated full-history nodes the shared MDBX allowance -- */

		/* START ALPHA CHANGE 2496 -- expose the event-log override only for full-history nodes -- */
		blockRangeEnv = fmt.Sprintf(
			"INNBC_JSONRPC_BLOCK_RANGE_LIMIT=%d\n",
			jsonrpc.DefaultBlockRangeLimit,
		)
		/* FINISH ALPHA CHANGE 2496 -- expose the event-log override only for full-history nodes -- */
		/* START ALPHA CHANGE 2461 -- expose fee-history overrides only for generated full-history nodes -- */
		feeHistoryEnv = fmt.Sprintf(
			"INNBC_JSONRPC_FEE_HISTORY_BLOCK_LIMIT=%d\n"+
				"INNBC_JSONRPC_FEE_HISTORY_PERCENTILE_LIMIT=%d\n"+
				"INNBC_JSONRPC_FEE_HISTORY_RESULT_LIMIT=%d\n",
			gasprice.DefaultFeeHistoryBlockLimit,
			gasprice.DefaultFeeHistoryPercentileLimit,
			gasprice.DefaultFeeHistoryResultLimit,
		)
		/* FINISH ALPHA CHANGE 2461 -- expose fee-history overrides only for generated full-history nodes -- */
		snapshotSourceDir = filepath.Join(
			snapshotRoot,
			"checkpoint",
		)
	}
	/* FINISH ALPHA CHANGE 1968 -- generate snapshot sources from the shared snapshot root -- */
	/* FINISH ALPHA CHANGE 1768 -- generate role-specific history and snapshot settings -- */

	if mode == externalValidatorMode {
		libp2pAddr = fmt.Sprintf("0.0.0.0:%d", libp2pPort)
		peerDirectoryURL = "https://innbcchain.io/api/v1/peers/exchange"
	}

	content := fmt.Sprintf(`# START ALPHA CHANGE 753 -- generated INNBC node service environment --
# START ALPHA CHANGE 3381 -- document read-only inspection in newly generated node environments --
# After provisioning, run node info nodeN or node info all with the same
# --base-dir and --config-dir used for creation or preparation.
# Inspection validates stored keys and selected environment structure.
# It prints public values only and does not repair incomplete slots.
# FINISH ALPHA CHANGE 3381 -- document read-only inspection in newly generated node environments --
INNBC_CHAIN=%s
INNBC_DATA_DIR=%s
INNBC_JSONRPC_ADDR=127.0.0.1:%d
INNBC_GRPC_ADDR=127.0.0.1:%d
INNBC_LIBP2P_ADDR=%s
# START ALPHA CHANGE 1207 -- generated optional advertised peer addresses --
INNBC_NAT_ADDR=%s
INNBC_DNS_ADDR=%s
# FINISH ALPHA CHANGE 1207 -- generated optional advertised peer addresses --
# START ALPHA CHANGE 1169 -- generated JSON-RPC public safety limits --
%s%s# START ALPHA CHANGE 2275 -- generate native RPC admission defaults --
INNBC_JSONRPC_REQUEST_RATE=100
INNBC_JSONRPC_REQUEST_BURST=200
INNBC_JSONRPC_CALL_RATE=200
INNBC_JSONRPC_CALL_BURST=400
INNBC_JSONRPC_MAX_CONCURRENT=32
INNBC_JSONRPC_MAX_EXPENSIVE=4
INNBC_JSONRPC_MAX_BODY_BYTES=1048576
INNBC_JSONRPC_MAX_WEBSOCKETS=64
INNBC_JSONRPC_MAX_FILTERS=1024
# FINISH ALPHA CHANGE 2275 -- generate native RPC admission defaults --
# FINISH ALPHA CHANGE 1169 -- generated JSON-RPC public safety limits --
INNBC_PEER_DIRECTORY_ENABLED=true
INNBC_PEER_DIRECTORY_URL=%s
# START ALPHA CHANGE 851 -- generated node history mode defaults --
INNBC_HISTORY_MODE=%s
%s# START ALPHA CHANGE 1733 -- generate independent snapshot consumer defaults --
INNBC_SNAPSHOT_SOURCE_DIR=%s
INNBC_SNAPSHOT_BOOTSTRAP_REQUIRED=false
# FINISH ALPHA CHANGE 1733 -- generate independent snapshot consumer defaults --
# START ALPHA CHANGE 1793 -- generate one node-wide MDBX map size --
INNBC_MDBX_MAP_SIZE_GB=%d
# START ALPHA CHANGE 3476 -- generate the default database creation page size --
INNBC_MDBX_PAGE_SIZE_BYTES=4096
# FINISH ALPHA CHANGE 3476 -- generate the default database creation page size --
# FINISH ALPHA CHANGE 1793 -- generate one node-wide MDBX map size --
# FINISH ALPHA CHANGE 851 -- generated node history mode defaults --
# FINISH ALPHA CHANGE 753 -- generated INNBC node service environment --
`,
		createConfig.chainPath,
		filepath.Join(createConfig.baseDir, nodeName),
		jsonRPCPort,
		grpcPort,
		libp2pAddr,
		/* START ALPHA CHANGE 1208 -- render optional advertised peer addresses -- */
		natAddr,
		dnsAddr,
		/* FINISH ALPHA CHANGE 1208 -- render optional advertised peer addresses -- */
		/* START ALPHA CHANGE 2496 -- render optional historical query environment settings -- */
		blockRangeEnv,
		feeHistoryEnv,
		/* FINISH ALPHA CHANGE 2496 -- render optional historical query environment settings -- */
		peerDirectoryURL,
		historyMode,
		historyGeometry,
		snapshotSourceDir,
		/* START ALPHA CHANGE 1794 -- render one node-wide MDBX map size -- */
		mdbxMapSizeGB,
		/* FINISH ALPHA CHANGE 1794 -- render one node-wide MDBX map size -- */
	)

	/* START ALPHA CHANGE 3355 -- exclusively create environment files without following occupied destinations -- */
	file, err := os.OpenFile(
		envPath,
		os.O_WRONLY|os.O_CREATE|os.O_EXCL,
		0600,
	)
	if errors.Is(err, os.ErrExist) {
		return fmt.Errorf("node env file already exists: %s: %w", envPath, err)
	}
	if err != nil {
		return fmt.Errorf("failed to create node env file %s: %w", envPath, err)
	}

	_, writeErr := file.WriteString(content)
	closeErr := file.Close()

	if writeErr != nil {
		return fmt.Errorf("failed to write node env file %s: %w", envPath, writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close node env file %s: %w", envPath, closeErr)
	}

	return nil
	/* FINISH ALPHA CHANGE 3355 -- exclusively create environment files without following occupied destinations -- */
}

func printNodeIdentity(action, nodeName, nodeDir string, nodeNumber int) error {
	/* START ALPHA CHANGE 3356 -- validate identity output ports before accessing storage -- */
	_, _, libp2pPort, err := nodePorts(nodeNumber)
	if err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3356 -- validate identity output ports before accessing storage -- */

	manager, err := secretshelper.SetupLocalSecretsManager(nodeDir)
	if err != nil {
		return err
	}

	nodeID, err := secretshelper.LoadNodeID(manager)
	if err != nil {
		return err
	}

	account, err := wallet.NewAccountFromSecret(manager)
	if err != nil {
		return err
	}

	address := account.Address()
	blsPublicKey := hex.EncodeToHex(account.Bls.PublicKey().Marshal())

	/* START ALPHA CHANGE 3356 -- use the already validated identity output port -- */
	fmt.Printf("\n[INNBC NODE %s]\n", action)
	/* FINISH ALPHA CHANGE 3356 -- use the already validated identity output port -- */
	fmt.Printf("Name|%s\n", nodeName)
	fmt.Printf("Data directory|%s\n", nodeDir)
	fmt.Printf("Env file|%s\n", filepath.Join(createConfig.configDir, nodeName+".env"))
	fmt.Printf("Validator address|%s\n", address.String())
	fmt.Printf("BLS public key|%s\n", blsPublicKey)
	fmt.Printf("Libp2p peer ID|%s\n", nodeID)
	fmt.Printf("Private-local multiaddr|/ip4/127.0.0.1/tcp/%d/p2p/%s\n", libp2pPort, nodeID)

	return nil
}

/* START ALPHA CHANGE 773 -- parse explicit node names for public node info lookup -- */
func parseNodeNumber(nodeName string) (int, error) {
	if !strings.HasPrefix(nodeName, "node") {
		return 0, fmt.Errorf("node name must use node<number> format, got %q", nodeName)
	}

	/* START ALPHA CHANGE 3354 -- validate node names independently from the creation batch limit -- */
	nodeNumber, err := strconv.Atoi(strings.TrimPrefix(nodeName, "node"))
	if err != nil || nodeNumber < 1 || nodeNumber > maxNodeNumber {
		return 0, fmt.Errorf("node name must use node<number> from node1 to node%d, got %q", maxNodeNumber, nodeName)
	}
	/* FINISH ALPHA CHANGE 3354 -- validate node names independently from the creation batch limit -- */

	return nodeNumber, nil
}

/* FINISH ALPHA CHANGE 773 -- parse explicit node names for public node info lookup -- */

/* START ALPHA CHANGE 3354 -- plan available identity and environment slots within checked numbering limits -- */
func nodePorts(nodeNumber int) (jsonRPC, grpc, libp2p int, err error) {
	if nodeNumber < 1 || nodeNumber > maxNodeNumber {
		return 0, 0, 0, fmt.Errorf(
			"node number must be between 1 and %d, got %d",
			maxNodeNumber,
			nodeNumber,
		)
	}

	return 8544 + nodeNumber, 9631 + nodeNumber, 30300 + nodeNumber, nil
}

func nodePathOccupied(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}

	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	return false, fmt.Errorf("unable to inspect node allocation path %s: %w", path, err)
}

func planNodeCreation(baseDir, configDir string, count int) ([]int, error) {
	if count < 1 || count > maxCreateCount {
		return nil, fmt.Errorf("node count must be between 1 and %d", maxCreateCount)
	}

	available := make([]int, 0, count)

	for nodeNumber := 1; nodeNumber <= maxNodeNumber; nodeNumber++ {
		nodeName := fmt.Sprintf("node%d", nodeNumber)

		identityOccupied, err := nodePathOccupied(filepath.Join(baseDir, nodeName))
		if err != nil {
			return nil, err
		}

		envOccupied, err := nodePathOccupied(filepath.Join(configDir, nodeName+".env"))
		if err != nil {
			return nil, err
		}

		if identityOccupied || envOccupied {
			continue
		}

		available = append(available, nodeNumber)
		if len(available) == count {
			return available, nil
		}
	}

	slotWord := "slots"
	verb := "remain"
	if len(available) == 1 {
		slotWord = "slot"
		verb = "remains"
	}

	return nil, fmt.Errorf(
		"requested %d new nodes, but only %d available %s %s within node1-node%d",
		count,
		len(available),
		slotWord,
		verb,
		maxNodeNumber,
	)
}

/* FINISH ALPHA CHANGE 3354 -- plan available identity and environment slots within checked numbering limits -- */

func getServiceCommand(action, short string) *cobra.Command {
	return &cobra.Command{
		Use:   action + " <node-name|all>",
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return runServiceCommand(action, args[0])
		},
	}
}

func runServiceCommand(action, target string) error {
	nodeNames, err := resolveNodeTargets(target)
	if err != nil {
		return err
	}

	for _, nodeName := range nodeNames {
		if err := runSystemctl(action, nodeName); err != nil {
			return err
		}
	}

	return nil
}

func resolveNodeTargets(target string) ([]string, error) {
	if target != "all" {
		return []string{target}, nil
	}

	matches, err := filepath.Glob(filepath.Join(defaultConfigDir, "node*.env"))
	if err != nil {
		return nil, err
	}

	nodeNames := make([]string, 0, len(matches))
	for _, match := range matches {
		nodeNames = append(nodeNames, strings.TrimSuffix(filepath.Base(match), ".env"))
	}

	sort.Strings(nodeNames)

	if len(nodeNames) == 0 {
		return nil, fmt.Errorf("no node env files found in %s", defaultConfigDir)
	}

	return nodeNames, nil
}

func runSystemctl(action, nodeName string) error {
	systemctlArgs := []string{action, "innbc-node@" + nodeName}

	switch action {
	case "start":
		systemctlArgs = []string{"enable", "--now", "innbc-node@" + nodeName}
	case "disable":
		systemctlArgs = []string{"disable", "--now", "innbc-node@" + nodeName}
	case "status":
		systemctlArgs = []string{"status", "innbc-node@" + nodeName, "--no-pager"}
	}

	cmd := exec.Command("systemctl", systemctlArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Fprintf(os.Stdout, "systemctl %s\n", strings.Join(systemctlArgs, " "))

	return cmd.Run()
}
