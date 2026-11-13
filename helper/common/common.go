package common

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"math/big"
	"os"
	"os/signal"

	/* START ALPHA CHANGE 3311 -- inspect retired clock configuration fields -- */
	"strings"
	/* FINISH ALPHA CHANGE 3311 -- inspect retired clock configuration fields -- */
	"syscall"

	/*START ALPHA CHANGE 28 -- remove Unix-only imports from cross-platform common file --*/
	"path/filepath"
	"time"

	/*FINISH ALPHA CHANGE 28 -- remove Unix-only imports from cross-platform common file --*/

	"github.com/innovativebioresearch/innbc-chain/helper/hex"
	"github.com/sethvargo/go-retry"
)

var (
	// MaxSafeJSInt represents max value which JS support
	// It is used for smartContract fields
	// Our staking repo is written in JS, as are many other clients
	// If we use higher value JS will not be able to parse it
	MaxSafeJSInt = uint64(math.Pow(2, 53) - 2)

	errInvalidDuration = errors.New("invalid duration")

	IBFTImportantNotice = "***** IMPORTANT NOTICE: IBFT consensus protocol is about to be" +
		" removed from the Edge in the following release. *****\n" +
		"Please migrate to the INNBCBFT protocol and plan your activities accordingly.\n" +
		"More information on how to execute the migration process can be found here" +
		"(https://wiki.polygon.technology/docs/edge/operate/ibft-to-innbcbft/)."
)

// RetryForever will execute a function until it completes without error or
// the context is cancelled or expired.
func RetryForever(ctx context.Context, interval time.Duration, fn func(context.Context) error) {
	_ = retry.Do(ctx, retry.NewConstant(interval), func(context.Context) error {
		// Execute function and end retries if no error or context done
		err := fn(ctx)
		if err == nil || IsContextDone(err) {
			return nil
		}

		// Retry on all other errors
		return retry.RetryableError(err)
	})
}

// IsContextDone returns true if the error is due to the context being cancelled
// or expired. This is useful for determining if a function should retry.
func IsContextDone(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// Min returns the strictly lower number
func Min(a, b uint64) uint64 {
	if a < b {
		return a
	}

	return b
}

// Max returns the strictly bigger number
func Max(a, b uint64) uint64 {
	if a > b {
		return a
	}

	return b
}

// BigMin returns the smallest of x or y.
func BigMin(x, y *big.Int) *big.Int {
	if x.Cmp(y) > 0 {
		return y
	}

	return x
}

func ConvertUnmarshalledUint(x interface{}) (uint64, error) {
	switch tx := x.(type) {
	case float64:
		/*START ALPHA CHANGE 26 -- reject negative JSON numbers before converting to uint64 --*/
		roundedValue := roundFloat(tx)
		if roundedValue < 0 {
			return 0, errors.New("must be positive value")
		}

		return uint64(roundedValue), nil
		/*FINISH ALPHA CHANGE 26 -- reject negative JSON numbers before converting to uint64 --*/
	case string:
		v, err := ParseUint64orHex(&tx)
		if err != nil {
			return 0, err
		}

		return v, nil
	default:
		return 0, errors.New("unsupported type for unmarshalled integer")
	}
}

func roundFloat(num float64) int64 {
	return int64(num + math.Copysign(0.5, num))
}

// SetupDataDir sets up the data directory and the corresponding sub-directories
func SetupDataDir(dataDir string, paths []string, perms fs.FileMode) error {
	if err := CreateDirSafe(dataDir, perms); err != nil {
		return fmt.Errorf("failed to create data dir: (%s): %w", dataDir, err)
	}

	for _, path := range paths {
		path := filepath.Join(dataDir, path)
		if err := CreateDirSafe(path, perms); err != nil {
			return fmt.Errorf("failed to create path: (%s): %w", path, err)
		}
	}

	return nil
}

// DirectoryExists checks if the directory at the specified path exists
func DirectoryExists(directoryPath string) bool {
	// Check if path is empty
	if directoryPath == "" {
		return false
	}

	// Grab the absolute filepath
	pathAbs, err := filepath.Abs(directoryPath)
	if err != nil {
		return false
	}

	// Check if the directory exists, and that it's actually a directory if there is a hit
	if fileInfo, statErr := os.Stat(pathAbs); os.IsNotExist(statErr) || (fileInfo != nil && !fileInfo.IsDir()) {
		return false
	}

	return true
}

// Checks if the file at the specified path exists
func FileExists(filePath string) bool {
	// Check if path is empty
	if filePath == "" {
		return false
	}

	// Grab the absolute filepath
	pathAbs, err := filepath.Abs(filePath)
	if err != nil {
		return false
	}

	// Check if the file exists, and that it's actually a file if there is a hit
	if fileInfo, statErr := os.Stat(pathAbs); os.IsNotExist(statErr) || (fileInfo != nil && fileInfo.IsDir()) {
		return false
	}

	return true
}

// Creates a directory at path and with perms level permissions.
// If directory already exists, owner and permissions are verified.
func CreateDirSafe(path string, perms fs.FileMode) error {
	info, err := os.Stat(path)
	// check if an error occurred other than path not exists
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	// create directory if it does not exist
	if !DirectoryExists(path) {
		if err := os.MkdirAll(path, perms); err != nil {
			return err
		}

		return nil
	}

	// verify that existing directory's owner and permissions are safe
	return verifyFileOwnerAndPermissions(path, info, perms)
}

// Creates a file at path and with perms level permissions.
// If file already exists, owner and permissions are
// verified, and the file is overwritten.
func SaveFileSafe(path string, data []byte, perms fs.FileMode) error {
	info, err := os.Stat(path)
	// check if an error occurred other than path not exists
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if FileExists(path) {
		// verify that existing file's owner and permissions are safe
		if err := verifyFileOwnerAndPermissions(path, info, perms); err != nil {
			return err
		}
	}

	// create or overwrite the file
	return os.WriteFile(path, data, perms)
}

/*START ALPHA CHANGE 29 -- move platform-specific file ownership verification out of common.go --*/
/*FINISH ALPHA CHANGE 29 -- move platform-specific file ownership verification out of common.go --*/

// JSONNumber is the number represented in decimal or hex in json
type JSONNumber struct {
	Value uint64
}

func (d *JSONNumber) MarshalJSON() ([]byte, error) {
	// START ALPHA CHANGE 1018 -- use fmt.Appendf for JSON number marshaling --
	return fmt.Appendf(nil, `"%s"`, hex.EncodeUint64(d.Value)), nil
	// FINISH ALPHA CHANGE 1018 -- use fmt.Appendf for JSON number marshaling --
}

func (d *JSONNumber) UnmarshalJSON(data []byte) error {
	var rawValue interface{}
	if err := json.Unmarshal(data, &rawValue); err != nil {
		return err
	}

	val, err := ConvertUnmarshalledUint(rawValue)
	if err != nil {
		return err
	}

	/*START ALPHA CHANGE 27 -- remove impossible negative check after uint64 conversion --*/
	d.Value = val
	/*FINISH ALPHA CHANGE 27 -- remove impossible negative check after uint64 conversion --*/

	return nil
}

// Duration is a wrapper struct for time.Duration which implements json (un)marshaling
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var (
		v   interface{}
		err error
	)

	if err = json.Unmarshal(b, &v); err != nil {
		return err
	}

	switch value := v.(type) {
	case float64:
		d.Duration = time.Duration(value)

		return nil
	case string:
		d.Duration, err = time.ParseDuration(value)
		if err != nil {
			return err
		}

		return nil
	}

	return errInvalidDuration
}

/* START ALPHA CHANGE 3311 -- share checked chain-clock arithmetic and configuration decoding -- */

// BlockTimeSeconds validates and converts a whole-second block interval.
func BlockTimeSeconds(blockTime time.Duration) (uint64, error) {
	if blockTime < time.Second {
		return 0, fmt.Errorf("block time must be at least 1s, got %s", blockTime)
	}
	if blockTime%time.Second != 0 {
		return 0, fmt.Errorf(
			"block time must be a whole number of seconds, got %s",
			blockTime,
		)
	}

	return uint64(blockTime / time.Second), nil
}

// ExpectedBlockTimestamp advances chain time without consulting a local clock.
func ExpectedBlockTimestamp(parentTimestamp uint64, blockTime time.Duration) (uint64, error) {
	seconds, err := BlockTimeSeconds(blockTime)
	if err != nil {
		return 0, err
	}

	if seconds > math.MaxUint64-parentTimestamp {
		return 0, fmt.Errorf("block timestamp addition overflows uint64")
	}

	timestamp := parentTimestamp + seconds
	if parentTimestamp > math.MaxInt64 || timestamp > math.MaxInt64 {
		return 0, fmt.Errorf("block timestamp exceeds supported execution range")
	}

	return timestamp, nil
}

// RejectRetiredBlockTimeDrift rejects the retired setting by presence,
// including explicit zero and null values.
func RejectRetiredBlockTimeDrift(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}

	for name := range fields {
		if strings.EqualFold(name, "blockTimeDrift") {
			return fmt.Errorf(
				"configuration field %q is retired; timestamps use parent timestamp plus blockTime",
				name,
			)
		}
	}

	return nil
}

// DecodeBlockTimeConfig validates the clock settings of a consensus section.
func DecodeBlockTimeConfig(raw []byte) (time.Duration, error) {
	if err := RejectRetiredBlockTimeDrift(raw); err != nil {
		return 0, err
	}

	var decoded struct {
		BlockTime Duration `json:"blockTime"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return 0, fmt.Errorf("invalid blockTime configuration: %w", err)
	}
	if _, err := BlockTimeSeconds(decoded.BlockTime.Duration); err != nil {
		return 0, fmt.Errorf("invalid blockTime configuration: %w", err)
	}

	return decoded.BlockTime.Duration, nil
}

/* FINISH ALPHA CHANGE 3311 -- share checked chain-clock arithmetic and configuration decoding -- */

// GetTerminationSignalCh returns a channel to emit signals by ctrl + c
func GetTerminationSignalCh() <-chan os.Signal {
	// wait for the user to quit with ctrl-c
	signalCh := make(chan os.Signal, 1)
	signal.Notify(
		signalCh,
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)

	return signalCh
}

// PadLeftOrTrim left-pads the passed in byte array to the specified size,
// or trims the array if it exceeds the passed in size
func PadLeftOrTrim(bb []byte, size int) []byte {
	l := len(bb)
	if l == size {
		return bb
	}

	if l > size {
		return bb[l-size:]
	}

	tmp := make([]byte, size)
	copy(tmp[size-l:], bb)

	return tmp
}

// ExtendByteSlice extends given byte slice by needLength parameter and trims it
func ExtendByteSlice(b []byte, needLength int) []byte {
	if n := needLength - len(b); n > 0 {
		b = append(b, make([]byte, n)...)
	}

	return b[:needLength]
}

// BigIntDivCeil performs integer division and rounds given result to next bigger integer number
// It is calculated using this formula result = (a + b - 1) / b
func BigIntDivCeil(a, b *big.Int) *big.Int {
	result := new(big.Int)

	return result.Add(a, b).
		Sub(result, big.NewInt(1)).
		Div(result, b)
}

// SafeAddUint64 sums two unsigned int64 numbers if there is no overflow.
// In case there is an overflow, it would return 0 and true, otherwise sum and false.
func SafeAddUint64(a, b uint64) (uint64, bool) {
	sum := a + b
	if sum < a || sum < b {
		return 0, true
	}

	return sum, false
}

// EncodeUint64ToBytes encodes provided uint64 to big endian byte slice
func EncodeUint64ToBytes(value uint64) []byte {
	result := make([]byte, 8)
	binary.BigEndian.PutUint64(result, value)

	return result
}

// EncodeBytesToUint64 big endian byte slice to uint64
func EncodeBytesToUint64(b []byte) uint64 {
	return binary.BigEndian.Uint64(b)
}
