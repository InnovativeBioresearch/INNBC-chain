package genesis

import (
	"encoding/hex"
	"errors"
	"fmt"

	/* START ALPHA CHANGE 3421 -- remove obsolete VSET amount dependency -- */
	/* FINISH ALPHA CHANGE 3421 -- remove obsolete VSET amount dependency -- */
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/hashicorp/go-hclog"
	"github.com/innovativebioresearch/innbc-chain/command"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft"

	/* START ALPHA CHANGE 2015 -- use shared proof generation and canonical genesis extra construction -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/signer"
	/* FINISH ALPHA CHANGE 2015 -- use shared proof generation and canonical genesis extra construction -- */
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/validator"
	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/wallet"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/secrets"
	"github.com/innovativebioresearch/innbc-chain/secrets/helper"
	"github.com/innovativebioresearch/innbc-chain/secrets/local"
)

const (
	StatError   = "StatError"
	ExistsError = "ExistsError"
)

// GenesisGenError is a specific error type for generating genesis
type GenesisGenError struct {
	message   string
	errorType string
}

// GetMessage returns the message of the genesis generation error
func (g *GenesisGenError) GetMessage() string {
	return g.message
}

// GetType returns the type of the genesis generation error
func (g *GenesisGenError) GetType() string {
	return g.errorType
}

// verifyGenesisExistence checks if the genesis file at the specified path is present
func verifyGenesisExistence(genesisPath string) *GenesisGenError {
	_, err := os.Stat(genesisPath)
	if err != nil && !os.IsNotExist(err) {
		return &GenesisGenError{
			message:   fmt.Sprintf("failed to stat (%s): %v", genesisPath, err),
			errorType: StatError,
		}
	}

	if !os.IsNotExist(err) {
		return &GenesisGenError{
			message:   fmt.Sprintf("genesis file at path (%s) already exists", genesisPath),
			errorType: ExistsError,
		}
	}

	return nil
}

/*START ALPHA CHANGE 22 -- remove unused parseTrackerStartBlocks helper reported by analyzer --*/
/*FINISH ALPHA CHANGE 22 -- remove unused parseTrackerStartBlocks helper reported by analyzer --*/

/* START ALPHA CHANGE 504 -- remove legacy burn-contract parser -- */
/* FINISH ALPHA CHANGE 504 -- remove legacy burn-contract parser -- */

type baseFeeInfo struct {
	baseFee            uint64
	baseFeeEM          uint64
	baseFeeChangeDenom uint64
}

// parseBaseFeeConfig parses provided base fee configuration and returns baseFeeInfo
func parseBaseFeeConfig(baseFeeConfigRaw string) (*baseFeeInfo, error) {
	baseFeeInfo := &baseFeeInfo{
		command.DefaultGenesisBaseFee,
		command.DefaultGenesisBaseFeeEM,
		command.DefaultGenesisBaseFeeChangeDenom,
	}

	baseFeeConfig := strings.Split(baseFeeConfigRaw, ":")
	if len(baseFeeConfig) > 3 {
		return baseFeeInfo, errors.New("invalid number of arguments for base fee configuration")
	}

	if len(baseFeeConfig) >= 1 && baseFeeConfig[0] != "" {
		baseFee, err := common.ParseUint64orHex(&baseFeeConfig[0])
		if err != nil {
			return baseFeeInfo, err
		}

		baseFeeInfo.baseFee = baseFee
	}

	if len(baseFeeConfig) >= 2 && baseFeeConfig[1] != "" {
		baseFeeEM, err := common.ParseUint64orHex(&baseFeeConfig[1])
		if err != nil {
			return baseFeeInfo, err
		}

		baseFeeInfo.baseFeeEM = baseFeeEM
	}

	if len(baseFeeConfig) == 3 && baseFeeConfig[2] != "" {
		baseFeeChangeDenom, err := common.ParseUint64orHex(&baseFeeConfig[2])
		if err != nil {
			return baseFeeInfo, err
		}

		baseFeeInfo.baseFeeChangeDenom = baseFeeChangeDenom
	}

	return baseFeeInfo, nil
}

// GetValidatorKeyFiles returns file names which has validator secrets
func GetValidatorKeyFiles(rootDir, filePrefix string) ([]string, error) {
	if rootDir == "" {
		rootDir = "."
	}

	files, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, err
	}

	matchedFiles := 0
	fileNames := make([]string, len(files))

	for _, file := range files {
		fileName := file.Name()
		if file.IsDir() && strings.HasPrefix(fileName, filePrefix) {
			fileNames[matchedFiles] = fileName
			matchedFiles++
		}
	}
	// reslice to remove empty entries
	fileNames = fileNames[:matchedFiles]

	// we must sort files by number after the prefix not by name string
	sort.Slice(fileNames, func(i, j int) bool {
		first := strings.TrimPrefix(fileNames[i], filePrefix)
		second := strings.TrimPrefix(fileNames[j], filePrefix)
		num1, _ := strconv.Atoi(strings.TrimLeft(first, "-"))
		num2, _ := strconv.Atoi(strings.TrimLeft(second, "-"))

		return num1 < num2
	})

	return fileNames, nil
}

// ReadValidatorsByPrefix reads validators secrets on a given root directory and with given folder prefix
func ReadValidatorsByPrefix(dir, prefix string) ([]*validator.GenesisValidator, error) {
	validatorKeyFiles, err := GetValidatorKeyFiles(dir, prefix)
	if err != nil {
		return nil, err
	}

	validators := make([]*validator.GenesisValidator, len(validatorKeyFiles))

	for i, file := range validatorKeyFiles {
		path := filepath.Join(dir, file)

		/* START ALPHA CHANGE 3839 -- retain and require the existing validator networking identity -- */
		account, nodeID, err := getSecrets(path)
		if err != nil {
			return nil, err
		}

		if nodeID == "" {
			return nil, fmt.Errorf(
				"missing libp2p identity in validator directory %q",
				path,
			)
		}
		/* FINISH ALPHA CHANGE 3839 -- retain and require the existing validator networking identity -- */

		/* START ALPHA CHANGE 2016 -- generate genesis possession proofs from the local validator identity -- */
		operator := account.Address()
		proof, err := signer.MakeINNBCBLSPoP(account.Bls, operator)
		if err != nil {
			return nil, fmt.Errorf("failed to prove validator %s: %w", operator, err)
		}
		proofBytes, err := proof.Marshal()
		if err != nil {
			return nil, fmt.Errorf("failed to encode validator proof: %w", err)
		}

		/* START ALPHA CHANGE 3421 -- generate validator identities without VSET balances -- */
		/* START ALPHA CHANGE 3840 -- serialize peer identity for directory bootstrap authorization -- */
		validators[i] = &validator.GenesisValidator{
			MultiAddr: "/p2p/" + nodeID,
			Address:   operator,
			BlsKey:    hex.EncodeToString(account.Bls.PublicKey().Marshal()),
			BlsProof:  hex.EncodeToString(proofBytes),
		}
		/* FINISH ALPHA CHANGE 3840 -- serialize peer identity for directory bootstrap authorization -- */
		/* FINISH ALPHA CHANGE 3421 -- generate validator identities without VSET balances -- */
		/* FINISH ALPHA CHANGE 2016 -- generate genesis possession proofs from the local validator identity -- */
	}

	return validators, nil
}

func getSecrets(directory string) (*wallet.Account, string, error) {
	baseConfig := &secrets.SecretsManagerParams{
		Logger: hclog.NewNullLogger(),
		Extra: map[string]interface{}{
			secrets.Path: directory,
		},
	}

	/* START ALPHA CHANGE 3234 -- preserve genesis identity discovery without remote configuration types -- */
	localManager, err := local.SecretsManagerFactory(baseConfig)
	/* FINISH ALPHA CHANGE 3234 -- preserve genesis identity discovery without remote configuration types -- */
	if err != nil {
		return nil, "", fmt.Errorf("unable to instantiate local secrets manager, %w", err)
	}

	nodeID, err := helper.LoadNodeID(localManager)
	if err != nil {
		return nil, "", err
	}

	account, err := wallet.NewAccountFromSecret(localManager)
	if err != nil {
		return nil, "", err
	}

	return account, nodeID, err
}

// GenerateExtraDataPolyBft populates Extra with specific fields required for innbcbft consensus protocol
/* START ALPHA CHANGE 2017 -- share canonical genesis extra encoding with startup validation -- */
func GenerateExtraDataPolyBft(validators []*validator.ValidatorMetadata) ([]byte, error) {
	return innbcbft.BuildGenesisExtraData(validators), nil
}

/* FINISH ALPHA CHANGE 2017 -- share canonical genesis extra encoding with startup validation -- */
