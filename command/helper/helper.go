package helper

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"

	/* START ALPHA CHANGE 3160 -- import owned signal registration instead of the shutdown timer -- */
	"os"
	"os/signal"
	"strings"
	"syscall"

	/* FINISH ALPHA CHANGE 3160 -- import owned signal registration instead of the shutdown timer -- */

	"github.com/innovativebioresearch/innbc-chain/chain"
	"github.com/innovativebioresearch/innbc-chain/command"

	/* START ALPHA CHANGE 740 -- remove legacy IBFT operator proto import from shared helper -- */
	/* FINISH ALPHA CHANGE 740 -- remove legacy IBFT operator proto import from shared helper -- */
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/server"
	"github.com/innovativebioresearch/innbc-chain/server/proto"
	txpoolOp "github.com/innovativebioresearch/innbc-chain/txpool/proto"
	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/ryanuber/columnize"
	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var ErrBlockTrackerPollInterval = errors.New("block tracker poll interval must be greater than 0")

type ClientCloseResult struct {
	Message string `json:"message"`
}

func (r *ClientCloseResult) GetOutput() string {
	return r.Message
}

type IPBinding string

const (
	LocalHostBinding     IPBinding = "127.0.0.1"
	AllInterfacesBinding IPBinding = "0.0.0.0"
)

// HandleSignals is a helper method for handling signals sent to the console
// Like stop, error, etc.
func HandleSignals(
	closeFn func(),
	outputter command.OutputFormatter,
) error {
	return HandleSignalsOrError(closeFn, outputter, nil)
}

/* START ALPHA CHANGE 1462 -- coordinate signals and fatal runtime shutdown in one owner -- */
/* START ALPHA CHANGE 3160 -- own signal registration and wait for component cleanup without an outer timer -- */
func HandleSignalsOrError(
	closeFn func(),
	outputter command.OutputFormatter,
	fatalErrCh <-chan error,
) error {
	signalCh := make(chan os.Signal, 1)
	signal.Notify(
		signalCh,
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)
	defer signal.Stop(signalCh)

	return handleSignalsOrError(closeFn, outputter, fatalErrCh, signalCh)
}

// handleSignalsOrError separates shutdown coordination from process signal
// registration so tests can supply signals without affecting the test process.
func handleSignalsOrError(
	closeFn func(),
	outputter command.OutputFormatter,
	fatalErrCh <-chan error,
	signalCh <-chan os.Signal,
) error {
	select {
	case fatalErr, ok := <-fatalErrCh:
		if !ok || fatalErr == nil {
			fatalErr = errors.New("fatal runtime error channel closed without an error")
		}

		// Fatal consensus errors require complete orderly shutdown before the
		// command returns a nonzero result to the process supervisor.
		if closeFn != nil {
			closeFn()
		}

		return fmt.Errorf("fatal consensus error: %w", fatalErr)

	case sig := <-signalCh:
		closeMessage := fmt.Sprintf("\n[SIGNAL] Caught signal: %v\n", sig)
		closeMessage += "Gracefully shutting down client...\n"

		outputter.SetCommandResult(
			&ClientCloseResult{
				Message: closeMessage,
			},
		)
		outputter.WriteOutput()

		gracefulCh := make(chan struct{})
		go func() {
			if closeFn != nil {
				closeFn()
			}

			close(gracefulCh)
		}()

		// Component shutdown owns its timeouts. A permanently blocked
		// component can keep this wait pending until the operator intervenes.
		select {
		case <-signalCh:
			// A second signal deliberately abandons graceful waiting.
			return errors.New("shutdown by signal channel")
		case <-gracefulCh:
			return nil
		}
	}
}

/* FINISH ALPHA CHANGE 3160 -- own signal registration and wait for component cleanup without an outer timer -- */
/* FINISH ALPHA CHANGE 1462 -- coordinate signals and fatal runtime shutdown in one owner -- */

// FormatList formats a list, using a specific blank value replacement
func FormatList(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"

	return columnize.Format(in, columnConf)
}

// FormatKV formats key value pairs:
//
// Key = Value
//
// Key = <none>
func FormatKV(in []string) string {
	columnConf := columnize.DefaultConfig()
	columnConf.Empty = "<none>"
	columnConf.Glue = " = "

	return columnize.Format(in, columnConf)
}

// GetTxPoolClientConnection returns the TxPool operator client connection
func GetTxPoolClientConnection(address string) (
	txpoolOp.TxnPoolOperatorClient,
	error,
) {
	conn, err := GetGRPCConnection(address)
	if err != nil {
		return nil, err
	}

	return txpoolOp.NewTxnPoolOperatorClient(conn), nil
}

// GetSystemClientConnection returns the System operator client connection
func GetSystemClientConnection(address string) (
	proto.SystemClient,
	error,
) {
	conn, err := GetGRPCConnection(address)
	if err != nil {
		return nil, err
	}

	return proto.NewSystemClient(conn), nil
}

/* START ALPHA CHANGE 741 -- remove legacy IBFT operator client helper -- */
/* FINISH ALPHA CHANGE 741 -- remove legacy IBFT operator client helper -- */

// GetGRPCConnection returns a grpc client connection
func GetGRPCConnection(address string) (*grpc.ClientConn, error) {
	conn, err := grpc.Dial(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to server: %w", err)
	}

	return conn, nil
}

// GetGRPCAddress extracts the set GRPC address
func GetGRPCAddress(cmd *cobra.Command) string {
	if cmd.Flags().Changed(command.GRPCAddressFlagLEGACY) {
		// The legacy GRPC flag was set, use that value
		return cmd.Flag(command.GRPCAddressFlagLEGACY).Value.String()
	}

	return cmd.Flag(command.GRPCAddressFlag).Value.String()
}

// GetJSONRPCAddress extracts the set JSON-RPC address
func GetJSONRPCAddress(cmd *cobra.Command) string {
	return cmd.Flag(command.JSONRPCFlag).Value.String()
}

// GetJSONLogFormat extracts the set JSON Format flag
func GetJSONLogFormat(cmd *cobra.Command) bool {
	return cmd.Flag(command.JSONOutputFlag).Changed
}

// RegisterJSONOutputFlag registers the --json output setting for all child commands
func RegisterJSONOutputFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool(
		command.JSONOutputFlag,
		false,
		"get all outputs in json format (default false)",
	)
}

// RegisterGRPCAddressFlag registers the base GRPC address flag for all child commands
func RegisterGRPCAddressFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		command.GRPCAddressFlag,
		fmt.Sprintf("%s:%d", LocalHostBinding, server.DefaultGRPCPort),
		"the GRPC interface",
	)
}

// RegisterLegacyGRPCAddressFlag registers the legacy GRPC address flag for all child commands
func RegisterLegacyGRPCAddressFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		command.GRPCAddressFlagLEGACY,
		fmt.Sprintf("%s:%d", LocalHostBinding, server.DefaultGRPCPort),
		"the GRPC interface",
	)

	// Mark the legacy grpc flag as hidden
	_ = cmd.PersistentFlags().MarkHidden(command.GRPCAddressFlagLEGACY)
}

// ParseGRPCAddress parses the passed in GRPC address
func ParseGRPCAddress(grpcAddress string) (*net.TCPAddr, error) {
	return net.ResolveTCPAddr("tcp", grpcAddress)
}

// RegisterJSONRPCFlag registers the base JSON-RPC address flag for all child commands
func RegisterJSONRPCFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		command.JSONRPCFlag,
		fmt.Sprintf("http://%s:%d", AllInterfacesBinding, server.DefaultJSONRPCPort),
		"the JSON-RPC interface",
	)
}

// ParseJSONRPCAddress parses the passed in JSONRPC address
func ParseJSONRPCAddress(jsonrpcAddress string) (*url.URL, error) {
	return url.ParseRequestURI(jsonrpcAddress)
}

// ResolveAddr resolves the passed in TCP address
// The second param is the default ip to bind to, if no ip address is specified
func ResolveAddr(address string, defaultIP IPBinding) (*net.TCPAddr, error) {
	addr, err := net.ResolveTCPAddr("tcp", address)

	if err != nil {
		return nil, fmt.Errorf("failed to parse addr '%s': %w", address, err)
	}

	if addr.IP == nil {
		addr.IP = net.ParseIP(string(defaultIP))
	}

	return addr, nil
}

// WriteGenesisConfigToDisk writes the passed in configuration to a genesis file at the specified path
func WriteGenesisConfigToDisk(genesisConfig *chain.Chain, genesisPath string) error {
	/* START ALPHA CHANGE 3102 -- validate genesis allocations before serialization or file creation -- */
	if err := genesisConfig.ValidateGenesisAllocations(); err != nil {
		return fmt.Errorf("failed to generate genesis: %w", err)
	}
	/* FINISH ALPHA CHANGE 3102 -- validate genesis allocations before serialization or file creation -- */

	data, err := json.MarshalIndent(genesisConfig, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to generate genesis: %w", err)
	}

	if err := common.SaveFileSafe(genesisPath, data, 0660); err != nil {
		return fmt.Errorf("failed to write genesis: %w", err)
	}

	return nil
}

func SetRequiredFlags(cmd *cobra.Command, requiredFlags []string) {
	for _, requiredFlag := range requiredFlags {
		_ = cmd.MarkFlagRequired(requiredFlag)
	}
}

func ParseAmount(amount string) (*big.Int, error) {
	result, ok := new(big.Int).SetString(amount, 0)
	if !ok || result.Cmp(big.NewInt(0)) <= 0 {
		return nil, fmt.Errorf("amount %s should be numerical value greater than zero", amount)
	}

	return result, nil
}

/* START ALPHA CHANGE 834 -- remove unused EOA proxy admin validation helper -- */
/* FINISH ALPHA CHANGE 834 -- remove unused EOA proxy admin validation helper -- */

type PremineInfo struct {
	Address types.Address
	Amount  *big.Int
	Key     string // only used for tests
}

/* START ALPHA CHANGE 3129 -- validate raw premine addresses before permissive conversion -- */
// parsePremineInfo parses provided premine information and returns premine address and amount
func ParsePremineInfo(premineInfoRaw string) (*PremineInfo, error) {
	var (
		address types.Address
		amount  = command.DefaultPremineBalance
		key     string
		err     error
	)

	parts := strings.Split(premineInfoRaw, ":")

	if addressErr := types.IsValidAddress(parts[0]); addressErr != nil {
		return nil, fmt.Errorf("invalid premine address %q: %w", parts[0], addressErr)
	}

	if len(parts) > 1 { // <addr>:<balance>
		amount, err = common.ParseUint256orHex(&parts[1])
		if err != nil {
			return nil, fmt.Errorf("failed to parse amount %s: %w", parts[1], err)
		}

		address = types.StringToAddress(parts[0])

		if len(parts) == 3 { // <addr>:<balance>:<key>
			key = parts[2]
		}
	} else {
		// <addr>
		address = types.StringToAddress(premineInfoRaw)
	}

	return &PremineInfo{Address: address, Amount: amount, Key: key}, nil
}

/* FINISH ALPHA CHANGE 3129 -- validate raw premine addresses before permissive conversion -- */
