package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"log"
	"os"
	"path"
	"runtime"
	"strings"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi/artifact"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
)

const (
	extension = ".sol"
)

func main() {
	_, filename, _, _ := runtime.Caller(0) //nolint: dogsled
	currentPath := path.Dir(filename)
	scpath := path.Join(currentPath, "../../../../core-contracts/artifacts/contracts/")

	readContracts := []struct {
		Path string
		Name string
	}{
		/* START ALPHA CHANGE 471 -- remove legacy state sync artifact generation -- */
		/* FINISH ALPHA CHANGE 471 -- remove legacy state sync artifact generation -- */
		/* START ALPHA CHANGE 790 -- stop generating legacy NativeERC20 artifact -- */
		/* FINISH ALPHA CHANGE 790 -- stop generating legacy NativeERC20 artifact -- */
		/* START ALPHA CHANGE 273 -- generate INNBC native token artifact -- */
		{
			"child/INNBCNativeToken.sol",
			"INNBCNativeToken",
		},
		/* FINISH ALPHA CHANGE 273 -- generate INNBC native token artifact -- */
		/* START ALPHA CHANGE 482 -- remove legacy child ERC20 bridge token and predicate artifact generation -- */
		/* FINISH ALPHA CHANGE 482 -- remove legacy child ERC20 bridge token and predicate artifact generation -- */
		/* START ALPHA CHANGE 518 -- remove legacy child ERC721 and ERC1155 bridge artifact generation -- */
		/* FINISH ALPHA CHANGE 518 -- remove legacy child ERC721 and ERC1155 bridge artifact generation -- */
		{
			"child/System.sol",
			"System",
		},
		/* START ALPHA CHANGE 3761 -- stop embedding the unused standalone G2 contract artifact -- */
		{
			"common/BLS.sol",
			"BLS",
		},
		/* START ALPHA CHANGE 3774 -- stop generating the unused Merkle system artifact -- */
		/* FINISH ALPHA CHANGE 3774 -- stop generating the unused Merkle system artifact -- */
		/* FINISH ALPHA CHANGE 3761 -- stop embedding the unused standalone G2 contract artifact -- */
		/* START ALPHA CHANGE 519 -- remove legacy rootchain bridge and staking manager artifact generation -- */
		/* FINISH ALPHA CHANGE 519 -- remove legacy rootchain bridge and staking manager artifact generation -- */
		/* START ALPHA CHANGE 615 -- stop generating unused RootERC20 mock artifact -- */
		/* FINISH ALPHA CHANGE 615 -- stop generating unused RootERC20 mock artifact -- */
		/* START ALPHA CHANGE 520 -- remove unused ERC721 and ERC1155 mock artifact generation -- */
		/* FINISH ALPHA CHANGE 520 -- remove unused ERC721 and ERC1155 mock artifact generation -- */
		/* START ALPHA CHANGE 536 -- remove pre-funded epoch reward pool artifact generation -- */
		/* FINISH ALPHA CHANGE 536 -- remove pre-funded epoch reward pool artifact generation -- */
		{
			"child/validator/ValidatorSet.sol",
			"ValidatorSet",
		},
		/* START ALPHA CHANGE 63 -- include INNBC validator registry in contract artifact generation -- */
		{
			"innbc/INNBCValidatorRegistry.sol",
			"INNBCValidatorRegistry",
		},
		/* FINISH ALPHA CHANGE 63 -- include INNBC validator registry in contract artifact generation -- */
		/* START ALPHA CHANGE 240 -- include INNBC treasury DAO in contract artifact generation -- */
		{
			"innbc/INNBCTreasuryDAO.sol",
			"INNBCTreasuryDAO",
		},
		/* FINISH ALPHA CHANGE 240 -- include INNBC treasury DAO in contract artifact generation -- */
		/* START ALPHA CHANGE 264 -- include INNBC governance modules in contract artifact generation -- */
		{
			"innbc/INNBCValidatorGovernance.sol",
			"INNBCValidatorGovernance",
		},
		{
			"innbc/INNBCBlockRewardGovernance.sol",
			"INNBCBlockRewardGovernance",
		},
		/* START ALPHA CHANGE 953 -- include INNBC community reward pool in contract artifact generation -- */
		{
			"innbc/INNBCCommunityRewardPool.sol",
			"INNBCCommunityRewardPool",
		},
		/* FINISH ALPHA CHANGE 953 -- include INNBC community reward pool in contract artifact generation -- */
		/* FINISH ALPHA CHANGE 264 -- include INNBC governance modules in contract artifact generation -- */
		/* START ALPHA CHANGE 806 -- include INNBC fee burn vault in contract artifact generation -- */
		{
			"innbc/INNBCFeeBurnVault.sol",
			"INNBCFeeBurnVault",
		},
		/* FINISH ALPHA CHANGE 806 -- include INNBC fee burn vault in contract artifact generation -- */
		/* START ALPHA CHANGE 996 -- include split proxy admin governance and executor in artifact generation -- */
		{
			"innbc/INNBCProxyAdminGovernance.sol",
			"INNBCProxyAdminGovernance",
		},
		{
			"innbc/INNBCProxyAdminExecutor.sol",
			"INNBCProxyAdminExecutor",
		},
		/* FINISH ALPHA CHANGE 996 -- include split proxy admin governance and executor in artifact generation -- */
		/* START ALPHA CHANGE 483 -- remove legacy EIP1559 bridge burn artifact generation -- */
		/* FINISH ALPHA CHANGE 483 -- remove legacy EIP1559 bridge burn artifact generation -- */
		{
			"lib/GenesisProxy.sol",
			"GenesisProxy",
		},
		{
			"../@openzeppelin/contracts/proxy/transparent/TransparentUpgradeableProxy.sol",
			"TransparentUpgradeableProxy",
		},
	}

	str := `// This is auto-generated file. DO NOT EDIT.
package contractsapi

`

	for _, v := range readContracts {
		artifactBytes, err := artifact.ReadArtifactData(scpath, v.Path, getContractName(v.Path))
		if err != nil {
			log.Fatal(err)
		}

		dst := &bytes.Buffer{}
		if err = json.Compact(dst, artifactBytes); err != nil {
			log.Fatal(err)
		}

		str += fmt.Sprintf("var %sArtifact string = `%s`\n", v.Name, dst.String())
	}

	output, err := format.Source([]byte(str))
	if err != nil {
		fmt.Println(str)
		log.Fatal(err)
	}

	if err = common.SaveFileSafe(currentPath+"/../gen_sc_data.go", output, 0600); err != nil {
		log.Fatal(err)
	}
}

// getContractName extracts smart contract name from provided path
func getContractName(path string) string {
	pathSegments := strings.Split(path, string([]rune{os.PathSeparator}))
	nameSegment := pathSegments[len(pathSegments)-1]

	return strings.Split(nameSegment, extension)[0]
}
