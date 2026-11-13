/* START ALPHA CHANGE 1610 -- centralize chain data and node identity paths -- */
package datadir

import "path/filepath"

const (
	ChainDataDirName  = "chain-data"
	BlockchainDirName = "blockchain"
	TrieDirName       = "trie"
	ConsensusDirName  = "consensus"

	KeysDirName           = "keys"
	ValidatorECDSADirName = "validator-ecdsa"
	ValidatorBLSDirName   = "validator-bls"
	LibP2PDirName         = "libp2p"

	ValidatorKeyFileName    = "validator.key"
	ValidatorBLSKeyFileName = "validator-bls.key"
	LibP2PKeyFileName       = "libp2p.key"
)

func ChainData(root string) string {
	return filepath.Join(root, ChainDataDirName)
}

func Blockchain(root string) string {
	return filepath.Join(ChainData(root), BlockchainDirName)
}

func Trie(root string) string {
	return filepath.Join(ChainData(root), TrieDirName)
}

func Consensus(root string) string {
	return filepath.Join(ChainData(root), ConsensusDirName)
}

func Keys(root string) string {
	return filepath.Join(root, KeysDirName)
}

func ValidatorECDSAKeys(root string) string {
	return filepath.Join(Keys(root), ValidatorECDSADirName)
}

func ValidatorBLSKeys(root string) string {
	return filepath.Join(Keys(root), ValidatorBLSDirName)
}

func LibP2PKeys(root string) string {
	return filepath.Join(Keys(root), LibP2PDirName)
}

func ValidatorKey(root string) string {
	return filepath.Join(ValidatorECDSAKeys(root), ValidatorKeyFileName)
}

func ValidatorBLSKey(root string) string {
	return filepath.Join(ValidatorBLSKeys(root), ValidatorBLSKeyFileName)
}

func LibP2PKey(root string) string {
	return filepath.Join(LibP2PKeys(root), LibP2PKeyFileName)
}

/* FINISH ALPHA CHANGE 1610 -- centralize chain data and node identity paths -- */
