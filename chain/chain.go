package chain

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"os"

	"github.com/hashicorp/go-multierror"
	// ALPHA CHANGE 1000 -- removed ethgo usage from chain/chain.go

	"github.com/innovativebioresearch/innbc-chain/helper/common"
	"github.com/innovativebioresearch/innbc-chain/helper/hex"
	"github.com/innovativebioresearch/innbc-chain/types"
)

const (
	// GenesisBaseFeeEM is the initial base fee elasticity multiplier for EIP-1559 blocks.
	GenesisBaseFeeEM = 2

	// GenesisGasLimit is the default gas limit of the Genesis block.
	GenesisGasLimit uint64 = 4712388

	// GenesisDifficulty is the default difficulty of the Genesis block.
	GenesisDifficulty uint64 = 131072

	// BaseFeeChangeDenom is the value to bound the amount the base fee can change between blocks
	BaseFeeChangeDenom = uint64(8)
)

var (
	// START ALPHA CHANGE 1000 -- set INNBC genesis base fee to one wei --
	// GenesisBaseFee is the initial base fee for EIP-1559 blocks.
	GenesisBaseFee = uint64(1)
	// FINISH ALPHA CHANGE 1000 -- set INNBC genesis base fee to one wei --
)

// Chain is the blockchain chain configuration
type Chain struct {
	Name    string   `json:"name"`
	Genesis *Genesis `json:"genesis"`
	Params  *Params  `json:"params"`
	/* START ALPHA CHANGE 634 -- remove legacy bootnode list from chain config -- */
	/* FINISH ALPHA CHANGE 634 -- remove legacy bootnode list from chain config -- */
}

// Genesis specifies the header fields, state of a genesis block
type Genesis struct {
	Nonce      [8]byte                           `json:"nonce"`
	Timestamp  uint64                            `json:"timestamp"`
	ExtraData  []byte                            `json:"extraData,omitempty"`
	GasLimit   uint64                            `json:"gasLimit"`
	Difficulty uint64                            `json:"difficulty"`
	Mixhash    types.Hash                        `json:"mixHash"`
	Coinbase   types.Address                     `json:"coinbase"`
	Alloc      map[types.Address]*GenesisAccount `json:"alloc,omitempty"`
	BaseFee    uint64                            `json:"baseFee"`
	BaseFeeEM  uint64                            `json:"baseFeeEM"`

	// BaseFeeChangeDenom is the value to bound the amount the base fee can change between blocks
	BaseFeeChangeDenom uint64 `json:"baseFeeChangeDenom,omitempty"`

	// Override
	StateRoot types.Hash

	// Only for testing
	Number     uint64     `json:"number"`
	GasUsed    uint64     `json:"gasUsed"`
	ParentHash types.Hash `json:"parentHash"`
}

// GenesisHeader converts the initially defined genesis struct to a header
func (g *Genesis) GenesisHeader() *types.Header {
	stateRoot := types.EmptyRootHash

	if g.StateRoot != types.ZeroHash {
		stateRoot = g.StateRoot
	}

	head := &types.Header{
		Number:       g.Number,
		Nonce:        g.Nonce,
		Timestamp:    g.Timestamp,
		ParentHash:   g.ParentHash,
		ExtraData:    g.ExtraData,
		GasLimit:     g.GasLimit,
		GasUsed:      g.GasUsed,
		BaseFee:      g.BaseFee,
		Difficulty:   g.Difficulty,
		MixHash:      g.Mixhash,
		Miner:        g.Coinbase.Bytes(),
		StateRoot:    stateRoot,
		Sha3Uncles:   types.EmptyUncleHash,
		ReceiptsRoot: types.EmptyRootHash,
		TxRoot:       types.EmptyRootHash,
	}

	// Set default values if none are passed in
	if g.GasLimit == 0 {
		head.GasLimit = GenesisGasLimit
	}

	if g.Difficulty == 0 {
		head.Difficulty = GenesisDifficulty
	}

	return head
}

// Hash computes the genesis hash
func (g *Genesis) Hash() types.Hash {
	header := g.GenesisHeader()
	header.ComputeHash()

	return header.Hash
}

// MarshalJSON implements the json interface
func (g *Genesis) MarshalJSON() ([]byte, error) {
	type Genesis struct {
		Nonce              string                      `json:"nonce"`
		Timestamp          *string                     `json:"timestamp,omitempty"`
		ExtraData          *string                     `json:"extraData,omitempty"`
		GasLimit           *string                     `json:"gasLimit,omitempty"`
		Difficulty         *string                     `json:"difficulty,omitempty"`
		Mixhash            types.Hash                  `json:"mixHash"`
		Coinbase           types.Address               `json:"coinbase"`
		Alloc              *map[string]*GenesisAccount `json:"alloc,omitempty"`
		Number             *string                     `json:"number,omitempty"`
		GasUsed            *string                     `json:"gasUsed,omitempty"`
		ParentHash         types.Hash                  `json:"parentHash"`
		BaseFee            *string                     `json:"baseFee"`
		BaseFeeEM          *string                     `json:"baseFeeEM"`
		BaseFeeChangeDenom *string                     `json:"baseFeeChangeDenom"`
	}

	var enc Genesis
	enc.Nonce = hex.EncodeToHex(g.Nonce[:])

	enc.Timestamp = common.EncodeUint64(g.Timestamp)
	enc.ExtraData = common.EncodeBytes(g.ExtraData)

	enc.GasLimit = common.EncodeUint64(g.GasLimit)
	enc.Difficulty = common.EncodeUint64(g.Difficulty)
	enc.BaseFee = common.EncodeUint64(g.BaseFee)
	enc.BaseFeeEM = common.EncodeUint64(g.BaseFeeEM)
	enc.BaseFeeChangeDenom = common.EncodeUint64(g.BaseFeeChangeDenom)

	enc.Mixhash = g.Mixhash
	enc.Coinbase = g.Coinbase

	if g.Alloc != nil {
		alloc := make(map[string]*GenesisAccount, len(g.Alloc))
		for k, v := range g.Alloc {
			alloc[k.String()] = v
		}

		enc.Alloc = &alloc
	}

	enc.Number = common.EncodeUint64(g.Number)
	enc.GasUsed = common.EncodeUint64(g.GasUsed)
	enc.ParentHash = g.ParentHash

	return json.Marshal(&enc)
}

// UnmarshalJSON implements the json interface
func (g *Genesis) UnmarshalJSON(data []byte) error {
	type Genesis struct {
		Nonce              *string                    `json:"nonce"`
		Timestamp          *string                    `json:"timestamp"`
		ExtraData          *string                    `json:"extraData"`
		GasLimit           *string                    `json:"gasLimit"`
		Difficulty         *string                    `json:"difficulty"`
		Mixhash            *types.Hash                `json:"mixHash"`
		Coinbase           *types.Address             `json:"coinbase"`
		Alloc              map[string]*GenesisAccount `json:"alloc"`
		Number             *string                    `json:"number"`
		GasUsed            *string                    `json:"gasUsed"`
		ParentHash         *types.Hash                `json:"parentHash"`
		BaseFee            *string                    `json:"baseFee"`
		BaseFeeEM          *string                    `json:"baseFeeEM"`
		BaseFeeChangeDenom *string                    `json:"baseFeeChangeDenom"`
	}

	var dec Genesis
	if err := json.Unmarshal(data, &dec); err != nil {
		return err
	}

	var err, subErr error

	parseError := func(field string, subErr error) {
		err = multierror.Append(err, fmt.Errorf("%s: %w", field, subErr))
	}

	nonce, subErr := common.ParseUint64orHex(dec.Nonce)
	if subErr != nil {
		parseError("nonce", subErr)
	}

	binary.BigEndian.PutUint64(g.Nonce[:], nonce)

	g.Timestamp, subErr = common.ParseUint64orHex(dec.Timestamp)
	if subErr != nil {
		parseError("timestamp", subErr)
	}

	if dec.ExtraData != nil {
		g.ExtraData, subErr = common.ParseBytes(dec.ExtraData)
		if subErr != nil {
			parseError("extradata", subErr)
		}
	}

	if dec.GasLimit == nil {
		return fmt.Errorf("field 'gaslimit' is required")
	}

	g.GasLimit, subErr = common.ParseUint64orHex(dec.GasLimit)
	if subErr != nil {
		parseError("gaslimit", subErr)
	}

	g.Difficulty, subErr = common.ParseUint64orHex(dec.Difficulty)
	if subErr != nil {
		parseError("difficulty", subErr)
	}

	g.BaseFee, subErr = common.ParseUint64orHex(dec.BaseFee)
	if subErr != nil {
		parseError("baseFee", subErr)
	}

	g.BaseFeeEM, subErr = common.ParseUint64orHex(dec.BaseFeeEM)
	if subErr != nil {
		parseError("baseFeeEM", subErr)
	}

	g.BaseFeeChangeDenom, subErr = common.ParseUint64orHex(dec.BaseFeeChangeDenom)
	if subErr != nil {
		parseError("baseFeeChangeDenom", subErr)
	}

	if dec.Mixhash != nil {
		g.Mixhash = *dec.Mixhash
	}

	if dec.Coinbase != nil {
		g.Coinbase = *dec.Coinbase
	}

	/* START ALPHA CHANGE 3131 -- validate allocation keys and reject duplicate decoded genesis addresses -- */
	if dec.Alloc != nil {
		allocations := make(map[types.Address]*GenesisAccount, len(dec.Alloc))
		invalidAllocations := false

		for k, v := range dec.Alloc {
			if addressErr := types.IsValidAddress(k); addressErr != nil {
				parseError("alloc", fmt.Errorf(
					"invalid genesis allocation address %q: %w",
					k, addressErr,
				))
				invalidAllocations = true
				continue
			}

			address := types.StringToAddress(k)
			if _, exists := allocations[address]; exists {
				parseError("alloc", fmt.Errorf(
					"duplicate decoded genesis allocation address %s",
					address,
				))
				invalidAllocations = true
				continue
			}

			allocations[address] = v
		}

		// Publish the decoded allocation map only after every key is valid
		// and every decoded address is unique.
		if !invalidAllocations {
			g.Alloc = allocations
		}
	}
	/* FINISH ALPHA CHANGE 3131 -- validate allocation keys and reject duplicate decoded genesis addresses -- */

	g.Number, subErr = common.ParseUint64orHex(dec.Number)
	if subErr != nil {
		parseError("number", subErr)
	}

	g.GasUsed, subErr = common.ParseUint64orHex(dec.GasUsed)
	if subErr != nil {
		parseError("gasused", subErr)
	}

	if dec.ParentHash != nil {
		g.ParentHash = *dec.ParentHash
	}

	return err
}

// Genesis alloc

// GenesisAccount is an account in the state of the genesis block.
type GenesisAccount struct {
	Code       []byte                    `json:"code,omitempty"`
	Storage    map[types.Hash]types.Hash `json:"storage,omitempty"`
	Balance    *big.Int                  `json:"balance,omitempty"`
	Nonce      uint64                    `json:"nonce,omitempty"`
	PrivateKey []byte                    `json:"secretKey,omitempty"` // for tests
}

type genesisAccountEncoder struct {
	Code       *string                   `json:"code,omitempty"`
	Storage    map[types.Hash]types.Hash `json:"storage,omitempty"`
	Balance    *string                   `json:"balance"`
	Nonce      *string                   `json:"nonce,omitempty"`
	PrivateKey *string                   `json:"secretKey,omitempty"`
}

// ENCODING //

func (g *GenesisAccount) MarshalJSON() ([]byte, error) {
	obj := &genesisAccountEncoder{}

	if g.Code != nil {
		obj.Code = common.EncodeBytes(g.Code)
	}

	if len(g.Storage) != 0 {
		obj.Storage = g.Storage
	}

	if g.Balance != nil {
		obj.Balance = common.EncodeBigInt(g.Balance)
	}

	if g.Nonce != 0 {
		obj.Nonce = common.EncodeUint64(g.Nonce)
	}

	if g.PrivateKey != nil {
		obj.PrivateKey = common.EncodeBytes(g.PrivateKey)
	}

	return json.Marshal(obj)
}

// DECODING //

func (g *GenesisAccount) UnmarshalJSON(data []byte) error {
	type GenesisAccount struct {
		Code       *string                   `json:"code,omitempty"`
		Storage    map[types.Hash]types.Hash `json:"storage,omitempty"`
		Balance    *string                   `json:"balance"`
		Nonce      *string                   `json:"nonce,omitempty"`
		PrivateKey *string                   `json:"secretKey,omitempty"`
	}

	var dec GenesisAccount
	if err := json.Unmarshal(data, &dec); err != nil {
		return err
	}

	var err error

	var subErr error

	parseError := func(field string, subErr error) {
		err = multierror.Append(err, fmt.Errorf("%s: %w", field, subErr))
	}

	if dec.Code != nil {
		g.Code, subErr = common.ParseBytes(dec.Code)
		if subErr != nil {
			parseError("code", subErr)
		}
	}

	if dec.Storage != nil {
		g.Storage = dec.Storage
	}

	g.Balance, subErr = common.ParseUint256orHex(dec.Balance)
	if subErr != nil {
		parseError("balance", subErr)
	}

	g.Nonce, subErr = common.ParseUint64orHex(dec.Nonce)

	if subErr != nil {
		parseError("nonce", subErr)
	}

	if dec.PrivateKey != nil {
		g.PrivateKey, subErr = common.ParseBytes(dec.PrivateKey)
		if subErr != nil {
			parseError("privatekey", subErr)
		}
	}

	return err
}

/* START ALPHA CHANGE 3315 -- validate the INNBC chain clock before startup or publication -- */
func (c *Chain) ValidateChainClock() error {
	if c == nil || c.Params == nil {
		return fmt.Errorf("chain parameters are missing")
	}

	engine, exists := c.Params.Engine["innbcbft"]
	if !exists {
		return nil
	}

	raw, err := json.Marshal(engine)
	if err != nil {
		return fmt.Errorf("invalid INNBCBFT clock configuration: %w", err)
	}
	blockTime, err := common.DecodeBlockTimeConfig(raw)
	if err != nil {
		return err
	}

	// Configuration-only consumers can decode consensus parameters without
	// supplying genesis. Imported chains separately require genesis.
	if c.Genesis != nil {
		if _, err := common.ExpectedBlockTimestamp(c.Genesis.Timestamp, blockTime); err != nil {
			return fmt.Errorf("invalid genesis timestamp: %w", err)
		}
	}

	return nil
}

/* FINISH ALPHA CHANGE 3315 -- validate the INNBC chain clock before startup or publication -- */

func Import(chain string) (*Chain, error) {
	return ImportFromFile(chain)
}

// ImportFromFile imports a chain from a filepath
func ImportFromFile(filename string) (*Chain, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	return importChain(data)
}

func importChain(content []byte) (*Chain, error) {
	var chain *Chain

	if err := json.Unmarshal(content, &chain); err != nil {
		return nil, err
	}

	/* START ALPHA CHANGE 1248 -- validate imported genesis base fee invariants -- */
	if err := validateImportedChain(chain); err != nil {
		return nil, err
	}
	/* FINISH ALPHA CHANGE 1248 -- validate imported genesis base fee invariants -- */

	return chain, nil
}

/* START ALPHA CHANGE 1249 -- reject malformed imported chain configs before startup -- */
func validateImportedChain(chainConfig *Chain) error {
	if chainConfig == nil {
		return fmt.Errorf("chain config is empty")
	}

	if chainConfig.Genesis == nil {
		return fmt.Errorf("chain genesis config is missing")
	}

	if chainConfig.Params == nil {
		return fmt.Errorf("chain params config is missing")
	}

	/* START ALPHA CHANGE 3530 -- reject nonpositive chain IDs when importing configuration -- */
	if err := chainConfig.ValidateChainID(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3530 -- reject nonpositive chain IDs when importing configuration -- */

	/* START ALPHA CHANGE 3101 -- reject malformed genesis allocations during configuration import -- */
	if err := chainConfig.ValidateGenesisAllocations(); err != nil {
		return err
	}
	/* FINISH ALPHA CHANGE 3101 -- reject malformed genesis allocations during configuration import -- */

	if engines := chainConfig.Params.Engine; len(engines) != 1 {
		return fmt.Errorf("expected one consensus engine but found %d", len(engines))
	}

	if chainConfig.Genesis.BaseFee == 0 {
		return fmt.Errorf("genesis baseFee must be greater than 0")
	}

	if chainConfig.Genesis.BaseFeeEM == 0 {
		return fmt.Errorf("genesis baseFeeEM must be greater than 0")
	}

	if chainConfig.Genesis.BaseFeeChangeDenom == 0 {
		return fmt.Errorf("genesis baseFeeChangeDenom must be greater than 0")
	}

	effectiveGasLimit := chainConfig.Genesis.GasLimit
	if effectiveGasLimit == 0 {
		effectiveGasLimit = GenesisGasLimit
	}

	/* START ALPHA CHANGE 2053 -- validate imported state transaction gas capacity and target -- */
	if effectiveGasLimit < types.StateTransactionGasLimit {
		return fmt.Errorf(
			"genesis gasLimit must be at least mandatory state transaction gas limit %d",
			types.StateTransactionGasLimit,
		)
	}

	if chainConfig.Params.BlockGasTarget != 0 &&
		chainConfig.Params.BlockGasTarget < types.StateTransactionGasLimit {
		return fmt.Errorf(
			"block gas target must be zero or at least mandatory state transaction gas limit %d",
			types.StateTransactionGasLimit,
		)
	}
	/* FINISH ALPHA CHANGE 2053 -- validate imported state transaction gas capacity and target -- */

	if effectiveGasLimit < chainConfig.Genesis.BaseFeeEM {
		return fmt.Errorf("genesis gasLimit must be greater than or equal to baseFeeEM")
	}

	/* START ALPHA CHANGE 3315 -- preserve fork validation and validate imported chain-clock configuration -- */
	if err := chainConfig.ValidateLondonFix(); err != nil {
		return err
	}

	return chainConfig.ValidateChainClock()
	/* FINISH ALPHA CHANGE 3315 -- preserve fork validation and validate imported chain-clock configuration -- */
}

/* FINISH ALPHA CHANGE 1249 -- reject malformed imported chain configs before startup -- */

/* START ALPHA CHANGE 3530 -- require a positive chain ID during import and startup -- */

// ValidateChainID requires a positive chain ID.
// The int64 configuration representation provides the upper bound.
func (c *Chain) ValidateChainID() error {
	if c == nil || c.Params == nil {
		return fmt.Errorf("chain params config is missing")
	}

	if c.Params.ChainID <= 0 {
		return fmt.Errorf(
			"chain ID must be positive, got %d",
			c.Params.ChainID,
		)
	}

	return nil
}

/* FINISH ALPHA CHANGE 3530 -- require a positive chain ID during import and startup -- */

/* START ALPHA CHANGE 2390 -- validate the supported London fee correction schedule without rewriting it -- */

func (c *Chain) ValidateLondonFix() error {
	if c == nil || c.Params == nil || c.Params.Forks == nil {
		return fmt.Errorf(
			"INNBC requires londonfix activation at block 0: fork configuration is missing",
		)
	}

	fork, exists := (*c.Params.Forks)[LondonFix]
	if !exists {
		return fmt.Errorf(
			"INNBC requires londonfix activation at block 0: londonfix is missing",
		)
	}

	if fork.Block != 0 {
		return fmt.Errorf(
			"INNBC requires londonfix activation at block 0: configured activation is block %d",
			fork.Block,
		)
	}

	return nil
}

/* FINISH ALPHA CHANGE 2390 -- validate the supported London fee correction schedule without rewriting it -- */
