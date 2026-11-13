package contractsapi

import (
	"embed"
	"log"
	"path"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi/artifact"
)

const (
	testContractsDir = "test-contracts"
)

var (
	// core-contracts smart contracts
	// core-contracts smart contracts
	/* START ALPHA CHANGE 459 -- remove legacy bridge and rootchain artifact holders -- */
	/* FINISH ALPHA CHANGE 459 -- remove legacy bridge and rootchain artifact holders -- */
	/* START ALPHA CHANGE 3762 -- remove the unused standalone G2 artifact holder and decoder -- */
	/* START ALPHA CHANGE 3775 -- remove the retired Merkle artifact holder and decoder -- */
	BLS               *artifact.Artifact
	System            *artifact.Artifact
	ChildValidatorSet *artifact.Artifact
	/* FINISH ALPHA CHANGE 3775 -- remove the retired Merkle artifact holder and decoder -- */
	/* FINISH ALPHA CHANGE 3762 -- remove the unused standalone G2 artifact holder and decoder -- */
	/* START ALPHA CHANGE 787 -- remove legacy NativeERC20 artifact holder -- */
	/* FINISH ALPHA CHANGE 787 -- remove legacy NativeERC20 artifact holder -- */
	/* START ALPHA CHANGE 275 -- rename native ERC20 artifact holder -- */
	INNBCNativeToken *artifact.Artifact
	/* FINISH ALPHA CHANGE 275 -- rename native ERC20 artifact holder -- */
	/* START ALPHA CHANGE 460 -- remove legacy child bridge artifact holders -- */
	/* FINISH ALPHA CHANGE 460 -- remove legacy child bridge artifact holders -- */
	/* START ALPHA CHANGE 461 -- remove legacy rootchain manager artifact holders -- */
	/* FINISH ALPHA CHANGE 461 -- remove legacy rootchain manager artifact holders -- */
	/* START ALPHA CHANGE 538 -- remove pre-funded epoch reward pool artifact holder -- */
	/* FINISH ALPHA CHANGE 538 -- remove pre-funded epoch reward pool artifact holder -- */
	ValidatorSet *artifact.Artifact
	/* START ALPHA CHANGE 94 -- add INNBC validator registry artifact holder -- */
	INNBCValidatorRegistry *artifact.Artifact
	/* FINISH ALPHA CHANGE 94 -- add INNBC validator registry artifact holder -- */
	/* START ALPHA CHANGE 241 -- add INNBC treasury DAO artifact holder -- */
	INNBCTreasuryDAO *artifact.Artifact
	/* FINISH ALPHA CHANGE 241 -- add INNBC treasury DAO artifact holder -- */
	/* START ALPHA CHANGE 266 -- add INNBC governance module artifact holders -- */
	INNBCValidatorGovernance   *artifact.Artifact
	INNBCBlockRewardGovernance *artifact.Artifact
	/* START ALPHA CHANGE 955 -- add INNBC community reward pool artifact holder -- */
	INNBCCommunityRewardPool *artifact.Artifact
	/* FINISH ALPHA CHANGE 955 -- add INNBC community reward pool artifact holder -- */
	/* FINISH ALPHA CHANGE 266 -- add INNBC governance module artifact holders -- */
	/* START ALPHA CHANGE 808 -- add INNBC fee burn vault artifact holder -- */
	INNBCFeeBurnVault *artifact.Artifact
	/* FINISH ALPHA CHANGE 808 -- add INNBC fee burn vault artifact holder -- */
	/* START ALPHA CHANGE 994 -- add split proxy admin governance and executor artifact holders -- */
	INNBCProxyAdminGovernance *artifact.Artifact
	INNBCProxyAdminExecutor   *artifact.Artifact
	/* FINISH ALPHA CHANGE 994 -- add split proxy admin governance and executor artifact holders -- */
	/* START ALPHA CHANGE 522 -- remove unused ERC721 and ERC1155 mock artifact holders -- */
	/* FINISH ALPHA CHANGE 522 -- remove unused ERC721 and ERC1155 mock artifact holders -- */
	/* START ALPHA CHANGE 485 -- remove legacy EIP1559 bridge burn artifact holder -- */
	/* FINISH ALPHA CHANGE 485 -- remove legacy EIP1559 bridge burn artifact holder -- */
	GenesisProxy                *artifact.Artifact
	TransparentUpgradeableProxy *artifact.Artifact

	// test smart contracts
	//go:embed test-contracts/*
	testContracts embed.FS

	/* START ALPHA CHANGE 3256 -- remove the orphaned legacy metadata test artifact holder -- */
	/* FINISH ALPHA CHANGE 3256 -- remove the orphaned legacy metadata test artifact holder -- */
	/* START ALPHA CHANGE 617 -- remove unused RootERC20 mock artifact holder -- */
	/* FINISH ALPHA CHANGE 617 -- remove unused RootERC20 mock artifact holder -- */
	TestSimple *artifact.Artifact
	/* START ALPHA CHANGE 611 -- remove test reward-token artifact holder -- */
	/* FINISH ALPHA CHANGE 611 -- remove test reward-token artifact holder -- */
)

func init() {
	var err error

	/* START ALPHA CHANGE 462 -- remove legacy checkpoint, exit helper, and L2 state sender decoders -- */
	/* FINISH ALPHA CHANGE 462 -- remove legacy checkpoint, exit helper, and L2 state sender decoders -- */

	/* START ALPHA CHANGE 3762 -- remove the unused standalone G2 artifact holder and decoder -- */
	BLS, err = artifact.DecodeArtifact([]byte(BLSArtifact))
	if err != nil {
		log.Fatal(err)
	}

	/* START ALPHA CHANGE 3775 -- remove the retired Merkle artifact holder and decoder -- */
	/* FINISH ALPHA CHANGE 3775 -- remove the retired Merkle artifact holder and decoder -- */
	/* FINISH ALPHA CHANGE 3762 -- remove the unused standalone G2 artifact holder and decoder -- */

	/* START ALPHA CHANGE 463 -- remove legacy state sender, root predicate, and state receiver decoders -- */
	/* FINISH ALPHA CHANGE 463 -- remove legacy state sender, root predicate, and state receiver decoders -- */

	System, err = artifact.DecodeArtifact([]byte(SystemArtifact))
	if err != nil {
		log.Fatal(err)
	}

	/* START ALPHA CHANGE 465 -- remove legacy child bridge token and predicate decoders -- */
	/* FINISH ALPHA CHANGE 465 -- remove legacy child bridge token and predicate decoders -- */

	/* START ALPHA CHANGE 788 -- stop decoding legacy NativeERC20 artifact -- */
	/* FINISH ALPHA CHANGE 788 -- stop decoding legacy NativeERC20 artifact -- */

	/* START ALPHA CHANGE 276 -- decode INNBC native token artifact -- */
	INNBCNativeToken, err = artifact.DecodeArtifact([]byte(INNBCNativeTokenArtifact))
	if err != nil {
		log.Fatal(err)
	}
	/* FINISH ALPHA CHANGE 276 -- decode INNBC native token artifact -- */

	/* START ALPHA CHANGE 618 -- stop decoding unused RootERC20 mock artifact -- */
	/* FINISH ALPHA CHANGE 618 -- stop decoding unused RootERC20 mock artifact -- */

	/* START ALPHA CHANGE 523 -- remove unused ERC721 and ERC1155 mock artifact decoders -- */
	/* FINISH ALPHA CHANGE 523 -- remove unused ERC721 and ERC1155 mock artifact decoders -- */

	/* START ALPHA CHANGE 3256 -- stop loading the orphaned legacy metadata test artifact -- */
	/* FINISH ALPHA CHANGE 3256 -- stop loading the orphaned legacy metadata test artifact -- */

	TestSimple, err = artifact.DecodeArtifact(readTestContractContent("TestSimple.json"))
	if err != nil {
		log.Fatal(err)
	}

	/* START ALPHA CHANGE 612 -- stop loading deleted test reward-token artifact -- */
	/* FINISH ALPHA CHANGE 612 -- stop loading deleted test reward-token artifact -- */

	/* START ALPHA CHANGE 466 -- remove legacy rootchain manager decoders -- */
	/* FINISH ALPHA CHANGE 466 -- remove legacy rootchain manager decoders -- */

	/* START ALPHA CHANGE 539 -- remove pre-funded epoch reward pool artifact decoder -- */
	/* FINISH ALPHA CHANGE 539 -- remove pre-funded epoch reward pool artifact decoder -- */

	ValidatorSet, err = artifact.DecodeArtifact([]byte(ValidatorSetArtifact))
	if err != nil {
		log.Fatal(err)
	}

	/* START ALPHA CHANGE 95 -- decode INNBC validator registry artifact -- */
	INNBCValidatorRegistry, err = artifact.DecodeArtifact([]byte(INNBCValidatorRegistryArtifact))
	if err != nil {
		log.Fatal(err)
	}
	/* FINISH ALPHA CHANGE 95 -- decode INNBC validator registry artifact -- */

	/* START ALPHA CHANGE 242 -- decode INNBC treasury DAO artifact -- */
	INNBCTreasuryDAO, err = artifact.DecodeArtifact([]byte(INNBCTreasuryDAOArtifact))
	if err != nil {
		log.Fatal(err)
	}
	/* FINISH ALPHA CHANGE 242 -- decode INNBC treasury DAO artifact -- */

	/* START ALPHA CHANGE 267 -- decode INNBC governance module artifacts -- */
	INNBCValidatorGovernance, err = artifact.DecodeArtifact([]byte(INNBCValidatorGovernanceArtifact))
	if err != nil {
		log.Fatal(err)
	}

	INNBCBlockRewardGovernance, err = artifact.DecodeArtifact([]byte(INNBCBlockRewardGovernanceArtifact))
	if err != nil {
		log.Fatal(err)
	}

	/* START ALPHA CHANGE 956 -- decode INNBC community reward pool artifact -- */
	INNBCCommunityRewardPool, err = artifact.DecodeArtifact([]byte(INNBCCommunityRewardPoolArtifact))
	if err != nil {
		log.Fatal(err)
	}
	/* FINISH ALPHA CHANGE 956 -- decode INNBC community reward pool artifact -- */
	/* FINISH ALPHA CHANGE 267 -- decode INNBC governance module artifacts -- */

	/* START ALPHA CHANGE 809 -- decode INNBC fee burn vault artifact -- */
	INNBCFeeBurnVault, err = artifact.DecodeArtifact([]byte(INNBCFeeBurnVaultArtifact))
	if err != nil {
		log.Fatal(err)
	}
	/* FINISH ALPHA CHANGE 809 -- decode INNBC fee burn vault artifact -- */
	/* START ALPHA CHANGE 995 -- decode split proxy admin governance and executor artifacts -- */
	INNBCProxyAdminGovernance, err = artifact.DecodeArtifact([]byte(INNBCProxyAdminGovernanceArtifact))
	if err != nil {
		log.Fatal(err)
	}

	INNBCProxyAdminExecutor, err = artifact.DecodeArtifact([]byte(INNBCProxyAdminExecutorArtifact))
	if err != nil {
		log.Fatal(err)
	}
	/* FINISH ALPHA CHANGE 995 -- decode split proxy admin governance and executor artifacts -- */

	/* START ALPHA CHANGE 486 -- remove legacy EIP1559 bridge burn artifact decoder -- */
	/* FINISH ALPHA CHANGE 486 -- remove legacy EIP1559 bridge burn artifact decoder -- */

	GenesisProxy, err = artifact.DecodeArtifact([]byte(GenesisProxyArtifact))
	if err != nil {
		log.Fatal(err)
	}

	TransparentUpgradeableProxy, err = artifact.DecodeArtifact([]byte(TransparentUpgradeableProxyArtifact))
	if err != nil {
		log.Fatal(err)
	}
}

func readTestContractContent(contractFileName string) []byte {
	contractRaw, err := testContracts.ReadFile(path.Join(testContractsDir, contractFileName))
	if err != nil {
		log.Fatal(err)
	}

	return contractRaw
}
