/* START ALPHA CHANGE 3768 -- modernize generator interface aliases and replace-all calls -- */
package main

/* START ALPHA CHANGE 3436 -- generate bindings without importing their output package -- */
import (
	"bytes"
	"errors"
	"fmt"
	"go/format"
	"log"
	"path/filepath"
	"regexp"
	"runtime"
	"slices" // ALPHA CHANGE 3769 -- use the standard-library membership helper
	"strconv"
	"strings"
	"text/template"

	"github.com/umbracle/ethgo/abi"

	"github.com/innovativebioresearch/innbc-chain/consensus/innbcbft/contractsapi/artifact"
	"github.com/innovativebioresearch/innbc-chain/helper/common"
)

/* FINISH ALPHA CHANGE 3436 -- generate bindings without importing their output package -- */

const (
	abiTypeNameFormat  = "var %sABIType = abi.MustNewType(\"%s\")"
	eventNameFormat    = "%sEvent"
	functionNameFormat = "%sFn"
)

var (
	signatureFunctionFormat = regexp.MustCompile(`^(.*)\((.*)\)$`)
)

type generatedData struct {
	resultString []string
	structs      []string
}

/* START ALPHA CHANGE 3436 -- read compiled artifacts independently of generated Go bindings -- */
func loadBindingArtifact(contractPath, contractName string) *artifact.Artifact {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("cannot determine the binding generator source directory")
	}

	rootFolder := filepath.Join(
		filepath.Dir(sourceFile),
		"../../../../core-contracts/artifacts/contracts",
	)

	data, err := artifact.ReadArtifactData(rootFolder, contractPath, contractName)
	if err != nil {
		log.Fatalf("cannot read compiled artifact %s: %v", contractName, err)
	}

	decoded, err := artifact.DecodeArtifact(data)
	if err != nil {
		log.Fatalf("cannot decode compiled artifact %s: %v", contractName, err)
	}

	return decoded
}

/* FINISH ALPHA CHANGE 3436 -- read compiled artifacts independently of generated Go bindings -- */

func main() {
	cases := []struct {
		contractName        string
		artifact            *artifact.Artifact
		generateConstructor bool
		functions           []string
		events              []string
	}{
		/* START ALPHA CHANGE 474 -- remove legacy state sync, checkpoint, exit, and child ERC20 predicate bindings -- */
		/* FINISH ALPHA CHANGE 474 -- remove legacy state sync, checkpoint, exit, and child ERC20 predicate bindings -- */
		/* START ALPHA CHANGE 791 -- stop generating legacy NativeERC20 binding -- */
		/* FINISH ALPHA CHANGE 791 -- stop generating legacy NativeERC20 binding -- */
		/* START ALPHA CHANGE 274 -- generate INNBC native token initialize binding -- */
		{
			"INNBCNativeToken",
			/* START ALPHA CHANGE 3436 -- load the compiled native-token artifact directly -- */
			loadBindingArtifact("child/INNBCNativeToken.sol", "INNBCNativeToken"),
			/* FINISH ALPHA CHANGE 3436 -- load the compiled native-token artifact directly -- */
			false,
			[]string{
				"initialize",
			},
			[]string{},
		},
		/* FINISH ALPHA CHANGE 274 -- generate INNBC native token initialize binding -- */
		/* START ALPHA CHANGE 243 -- generate INNBC treasury DAO initialize binding -- */
		{
			"INNBCTreasuryDAO",
			/* START ALPHA CHANGE 3436 -- load the compiled treasury artifact directly -- */
			loadBindingArtifact("innbc/INNBCTreasuryDAO.sol", "INNBCTreasuryDAO"),
			/* FINISH ALPHA CHANGE 3436 -- load the compiled treasury artifact directly -- */
			false,
			[]string{
				"initialize",
			},
			[]string{},
		},
		/* FINISH ALPHA CHANGE 243 -- generate INNBC treasury DAO initialize binding -- */
		/* START ALPHA CHANGE 265 -- generate INNBC governance module initialize bindings -- */
		{
			"INNBCValidatorGovernance",
			/* START ALPHA CHANGE 3436 -- load the compiled validator-governance artifact directly -- */
			loadBindingArtifact("innbc/INNBCValidatorGovernance.sol", "INNBCValidatorGovernance"),
			/* FINISH ALPHA CHANGE 3436 -- load the compiled validator-governance artifact directly -- */
			false,
			[]string{
				"initialize",
			},
			[]string{},
		},
		{
			"INNBCBlockRewardGovernance",
			/* START ALPHA CHANGE 3436 -- load the compiled block-reward-governance artifact directly -- */
			loadBindingArtifact("innbc/INNBCBlockRewardGovernance.sol", "INNBCBlockRewardGovernance"),
			/* FINISH ALPHA CHANGE 3436 -- load the compiled block-reward-governance artifact directly -- */
			false,
			[]string{
				"initialize",
			},
			[]string{},
		},
		/* START ALPHA CHANGE 954 -- generate INNBC community reward pool initialize binding -- */
		{
			"INNBCCommunityRewardPool",
			/* START ALPHA CHANGE 3436 -- load the compiled community-pool artifact directly -- */
			loadBindingArtifact("innbc/INNBCCommunityRewardPool.sol", "INNBCCommunityRewardPool"),
			/* FINISH ALPHA CHANGE 3436 -- load the compiled community-pool artifact directly -- */
			false,
			[]string{
				"initialize",
			},
			[]string{},
		},
		/* FINISH ALPHA CHANGE 954 -- generate INNBC community reward pool initialize binding -- */
		/* FINISH ALPHA CHANGE 265 -- generate INNBC governance module initialize bindings -- */
		/* START ALPHA CHANGE 807 -- generate INNBC fee burn vault initialize binding -- */
		{
			"INNBCFeeBurnVault",
			/* START ALPHA CHANGE 3436 -- load the compiled fee-burn-vault artifact directly -- */
			loadBindingArtifact("innbc/INNBCFeeBurnVault.sol", "INNBCFeeBurnVault"),
			/* FINISH ALPHA CHANGE 3436 -- load the compiled fee-burn-vault artifact directly -- */
			false,
			[]string{
				"initialize",
			},
			[]string{},
		},
		/* FINISH ALPHA CHANGE 807 -- generate INNBC fee burn vault initialize binding -- */
		/* START ALPHA CHANGE 997 -- generate split proxy admin governance and executor bindings -- */
		{
			"INNBCProxyAdminGovernance",
			/* START ALPHA CHANGE 3436 -- load the compiled proxy-admin-governance artifact directly -- */
			loadBindingArtifact("innbc/INNBCProxyAdminGovernance.sol", "INNBCProxyAdminGovernance"),
			/* FINISH ALPHA CHANGE 3436 -- load the compiled proxy-admin-governance artifact directly -- */
			false,
			[]string{
				"initialize",
			},
			[]string{},
		},
		{
			"INNBCProxyAdminExecutor",
			/* START ALPHA CHANGE 3436 -- load the compiled proxy-admin-executor artifact directly -- */
			loadBindingArtifact("innbc/INNBCProxyAdminExecutor.sol", "INNBCProxyAdminExecutor"),
			/* FINISH ALPHA CHANGE 3436 -- load the compiled proxy-admin-executor artifact directly -- */
			false,
			[]string{
				"initialize",
			},
			[]string{},
		},
		/* FINISH ALPHA CHANGE 997 -- generate split proxy admin governance and executor bindings -- */
		/* START ALPHA CHANGE 475 -- remove legacy root ERC20 predicate binding -- */
		/* FINISH ALPHA CHANGE 475 -- remove legacy root ERC20 predicate binding -- */
		/* START ALPHA CHANGE 616 -- stop generating unused RootERC20 mock bindings -- */
		/* FINISH ALPHA CHANGE 616 -- stop generating unused RootERC20 mock bindings -- */
		/* START ALPHA CHANGE 521 -- remove unused ERC721 and ERC1155 mock binding generation -- */
		/* FINISH ALPHA CHANGE 521 -- remove unused ERC721 and ERC1155 mock binding generation -- */
		/* START ALPHA CHANGE 480 -- remove legacy rootchain staking manager bindings -- */
		/* FINISH ALPHA CHANGE 480 -- remove legacy rootchain staking manager bindings -- */
		/* START ALPHA CHANGE 3425 -- generate ValidatorSet bindings without retired VSET operations -- */
		{
			"ValidatorSet",
			/* START ALPHA CHANGE 3436 -- load the compiled epoch-tracking artifact directly -- */
			loadBindingArtifact("child/validator/ValidatorSet.sol", "ValidatorSet"),
			/* FINISH ALPHA CHANGE 3436 -- load the compiled epoch-tracking artifact directly -- */
			false,
			[]string{
				"commitEpoch",
				"initialize",
			},
			[]string{},
		},
		/* FINISH ALPHA CHANGE 3425 -- generate ValidatorSet bindings without retired VSET operations -- */
		/* START ALPHA CHANGE 537 -- remove pre-funded epoch reward pool binding generation -- */
		/* FINISH ALPHA CHANGE 537 -- remove pre-funded epoch reward pool binding generation -- */
		/* START ALPHA CHANGE 484 -- remove legacy EIP1559 bridge burn binding generation -- */
		/* FINISH ALPHA CHANGE 484 -- remove legacy EIP1559 bridge burn binding generation -- */
		{
			"GenesisProxy",
			/* START ALPHA CHANGE 3436 -- load the compiled genesis-proxy artifact directly -- */
			loadBindingArtifact("lib/GenesisProxy.sol", "GenesisProxy"),
			/* FINISH ALPHA CHANGE 3436 -- load the compiled genesis-proxy artifact directly -- */
			false,
			[]string{
				"protectSetUpProxy",
				"setUpProxy",
			},
			[]string{},
		},
		{
			"TransparentUpgradeableProxy",
			/* START ALPHA CHANGE 3436 -- load the compiled transparent-proxy artifact directly -- */
			loadBindingArtifact("../@openzeppelin/contracts/proxy/transparent/TransparentUpgradeableProxy.sol", "TransparentUpgradeableProxy"),
			/* FINISH ALPHA CHANGE 3436 -- load the compiled transparent-proxy artifact directly -- */
			true,
			[]string{},
			[]string{},
		},
	}

	generatedData := &generatedData{}

	for _, c := range cases {
		if c.generateConstructor {
			if err := generateConstructor(generatedData, c.contractName, c.artifact.Abi.Constructor); err != nil {
				log.Fatal(err)
			}
		}

		for _, methodRaw := range c.functions {
			// There could be two objects with the same name in the generated JSON ABI (hardhat bug).
			// This case can be fixed by specifying a function signature instead of just name
			// e.g. "myFunc(address,bool,uint256)" instead of just "myFunc"
			var (
				method              *abi.Method
				resolvedBySignature = false
			)

			if signatureFunctionFormat.MatchString(methodRaw) {
				method = c.artifact.Abi.GetMethodBySignature(methodRaw)
				resolvedBySignature = true
			} else {
				method = c.artifact.Abi.GetMethod(methodRaw)
			}

			if err := generateFunction(generatedData, c.contractName, method, resolvedBySignature); err != nil {
				log.Fatal(err)
			}
		}

		for _, event := range c.events {
			if err := generateEvent(generatedData, c.contractName, c.artifact.Abi.Events[event]); err != nil {
				log.Fatal(err)
			}
		}
	}

	/* START ALPHA CHANGE 3434 -- import ethgo only when generated bindings reference it -- */
	declarations := strings.Join(generatedData.resultString, "\n")

	str := `// Code generated by scapi/gen. DO NOT EDIT.
package contractsapi

import (
	"math/big"

	"github.com/innovativebioresearch/innbc-chain/types"
	"github.com/umbracle/ethgo/abi"
`
	if strings.Contains(declarations, "ethgo.") {
		str += "\t\"github.com/umbracle/ethgo\"\n"
	}

	str += ")\n\n" + declarations
	/* FINISH ALPHA CHANGE 3434 -- import ethgo only when generated bindings reference it -- */

	output, err := format.Source([]byte(str))
	if err != nil {
		fmt.Println(str)
		log.Fatal(err)
	}

	if err = common.SaveFileSafe("./consensus/innbcbft/contractsapi/contractsapi.go", output, 0600); err != nil {
		log.Fatal(err)
	}
}

func getInternalType(paramName string, paramAbiType *abi.Type) string {
	internalType := paramAbiType.InternalType()
	if internalType == "" {
		internalType = strings.Title(paramName)
	} else {
		internalType = strings.TrimSuffix(internalType, "[]")      // remove [] if it's struct array
		internalType = strings.TrimPrefix(internalType, "struct ") // remove struct prefix
		// if struct is taken from an interface (ICheckpoint.Validator), remove interface
		parts := strings.Split(internalType, ".")
		if len(parts) > 1 {
			internalType = parts[1]
		}
	}

	return internalType
}

// generateType generates code for structs used in smart contract functions and events
func generateType(generatedData *generatedData, name string, obj *abi.Type, res *[]string) (string, error) {
	if obj.Kind() != abi.KindTuple {
		return "", errors.New("type not expected")
	}

	internalType := getInternalType(name, obj)
	generatedData.structs = append(generatedData.structs, internalType)

	str := []string{
		"type " + internalType + " struct {",
	}

	for _, tupleElem := range obj.TupleElems() {
		elem := tupleElem.Elem

		var typ string

		if elem.Kind() == abi.KindTuple {
			// Struct
			nestedType, err := generateNestedType(generatedData, tupleElem.Name, elem, res)
			if err != nil {
				return "", err
			}

			typ = nestedType
		} else if elem.Kind() == abi.KindSlice && elem.Elem().Kind() == abi.KindTuple {
			// []Struct
			nestedType, err := generateNestedType(generatedData, getInternalType(tupleElem.Name, elem), elem.Elem(), res)
			if err != nil {
				return "", err
			}

			typ = "[]" + nestedType
		} else if elem.Kind() == abi.KindArray && elem.Elem().Kind() == abi.KindTuple {
			// [n]Struct
			nestedType, err := generateNestedType(generatedData, getInternalType(tupleElem.Name, elem), elem.Elem(), res)
			if err != nil {
				return "", err
			}

			typ = "[" + strconv.Itoa(elem.Size()) + "]" + nestedType
		} else if elem.Kind() == abi.KindAddress {
			// for address use the native `types.Address` type instead of `ethgo.Address`. Note that
			// this only works for simple types and not for []address inputs. This is good enough since
			// there are no kinds like that in our smart contracts.
			typ = "types.Address"
		} else {
			// for the rest of the types use the go type returned by abi
			typ = elem.GoType().String()
		}

		// []byte and [n]byte get rendered as []uint68 and [n]uint8, since we do not have any
		// uint8 internally in innbcbft, we can use regexp to replace those values with the
		// correct byte representation
		typ = strings.ReplaceAll(typ, "[32]uint8", "types.Hash")
		typ = strings.ReplaceAll(typ, "]uint8", "]byte")

		// Trim the leading _ from name if it exists
		fieldName := strings.TrimPrefix(tupleElem.Name, "_")

		// Replacement of Id for ID to make the linter happy
		fieldName = strings.Title(fieldName)
		fieldName = strings.ReplaceAll(fieldName, "Id", "ID")

		str = append(str, fmt.Sprintf("%s %s `abi:\"%s\"`", fieldName, typ, tupleElem.Name))
	}

	str = append(str, "}")
	*res = append(*res, strings.Join(str, "\n"))

	return internalType, nil
}

// generateNestedType generates code for nested types found in smart contracts structs
func generateNestedType(generatedData *generatedData, name string, obj *abi.Type, res *[]string) (string, error) {
	/* START ALPHA CHANGE 3769 -- simplify nested-type membership checking with slices.Contains -- */
	if slices.Contains(generatedData.structs, name) {
		// Reuse the existing type when multiple functions reference the same struct.
		return "*" + name, nil
	}
	/* FINISH ALPHA CHANGE 3769 -- simplify nested-type membership checking with slices.Contains -- */

	result, err := generateType(generatedData, name, obj, res)
	if err != nil {
		return "", err
	}

	*res = append(*res, fmt.Sprintf(abiTypeNameFormat, result, obj.Format(true)))

	nestedTypeFunctions, err := generateAbiFuncsForNestedType(result)
	if err != nil {
		return "", err
	}

	*res = append(*res, nestedTypeFunctions)

	return "*" + result, nil
}

// generateAbiFuncsForNestedType generates necessary functions for nested types smart contracts interaction
func generateAbiFuncsForNestedType(name string) (string, error) {
	tmpl := `func ({{.Sig}} *{{.TName}}) EncodeAbi() ([]byte, error) {
		return {{.Name}}ABIType.Encode({{.Sig}})
	}
	
	func ({{.Sig}} *{{.TName}}) DecodeAbi(buf []byte) error {
		return decodeStruct({{.Name}}ABIType, buf, &{{.Sig}})
	}`

	title := strings.Title(name)

	inputs := map[string]any{
		"Sig":   strings.ToLower(string(name[0])),
		"Name":  title,
		"TName": title,
	}

	return renderTmpl(tmpl, inputs)
}

// generateEvent generates code for smart contract events
func generateEvent(generatedData *generatedData, contractName string, event *abi.Event) error {
	name := fmt.Sprintf(eventNameFormat, event.Name)
	res := []string{}

	_, err := generateType(generatedData, name, event.Inputs, &res)
	if err != nil {
		return err
	}

	// write encode/decode functions
	tmplStr := `
{{range .Structs}}
	{{.}}
{{ end }}

func (*{{.TName}}) Sig() ethgo.Hash {
	return {{.ContractName}}.Abi.Events["{{.Name}}"].ID()
}

func ({{.Sig}} *{{.TName}}) Encode() ([]byte, error) {
	return {{.ContractName}}.Abi.Events["{{.Name}}"].Inputs.Encode({{.Sig}})
}

/* START ALPHA CHANGE 3767 -- preserve generated event decoding without the unused wrapper -- */
func ({{.Sig}} *{{.TName}}) ParseLog(log *ethgo.Log) (bool, error) {
	if (!{{.ContractName}}.Abi.Events["{{.Name}}"].Match(log)) {
		return false, nil
	}

	val, err := {{.ContractName}}.Abi.Events["{{.Name}}"].ParseLog(log)
	if err != nil {
		return true, err
	}

	return true, decodeImpl(val, {{.Sig}})
}
/* FINISH ALPHA CHANGE 3767 -- preserve generated event decoding without the unused wrapper -- */

func ({{.Sig}} *{{.TName}}) Decode(input []byte) error {
	return {{.ContractName}}.Abi.Events["{{.Name}}"].Inputs.DecodeStruct(input, &{{.Sig}})
}
`

	inputs := map[string]any{
		"Structs":      res,
		"Sig":          strings.ToLower(string(name[0])),
		"Name":         event.Name,
		"TName":        strings.Title(name),
		"ContractName": contractName,
	}

	renderedString, err := renderTmpl(tmplStr, inputs)
	if err != nil {
		return err
	}

	generatedData.resultString = append(generatedData.resultString, renderedString)

	return nil
}

// generateConstruct generates stubs for a smart contract constructor
func generateConstructor(generatedData *generatedData,
	contractName string, constructor *abi.Method) error {
	methodName := fmt.Sprintf(functionNameFormat, strings.Title(contractName+"Constructor"))
	res := []string{}

	_, err := generateType(generatedData, methodName, constructor.Inputs, &res)
	if err != nil {
		return err
	}

	// write encode/decode functions
	tmplStr := `
{{range .Structs}}
	{{.}}
{{ end }}

func ({{.Sig}} *{{.TName}}) Sig() []byte {
	return {{.ContractName}}.Abi.Constructor.ID()
}

func ({{.Sig}} *{{.TName}}) EncodeAbi() ([]byte, error) {
	return {{.ContractName}}.Abi.Constructor.Inputs.Encode({{.Sig}})
}

func ({{.Sig}} *{{.TName}}) DecodeAbi(buf []byte) error {
	return decodeMethod({{.ContractName}}.Abi.Constructor, buf, {{.Sig}})
}`

	inputs := map[string]any{
		"Structs":      res,
		"Sig":          strings.ToLower(string(methodName[0])),
		"ContractName": contractName,
		"TName":        strings.Title(methodName),
	}

	renderedString, err := renderTmpl(tmplStr, inputs)
	if err != nil {
		return err
	}

	generatedData.resultString = append(generatedData.resultString, renderedString)

	return nil
}

// generateFunction generates code for smart contract function and its parameters
func generateFunction(generatedData *generatedData, contractName string,
	method *abi.Method, fnSigResolution bool) error {
	methodName := fmt.Sprintf(functionNameFormat, strings.Title(method.Name+contractName))
	res := []string{}

	_, err := generateType(generatedData, methodName, method.Inputs, &res)
	if err != nil {
		return err
	}

	// write encode/decode functions

	tmplString := `
	{{range .Structs}}
		{{.}}
	{{ end }}
	
	func ({{.Sig}} *{{.TName}}) Sig() []byte {
		return {{.ContractName}}.Abi.{{.MethodGetter}}["{{.Name}}"].ID()
	}
	
	func ({{.Sig}} *{{.TName}}) EncodeAbi() ([]byte, error) {
		return {{.ContractName}}.Abi.{{.MethodGetter}}["{{.Name}}"].Encode({{.Sig}})
	}
	
	func ({{.Sig}} *{{.TName}}) DecodeAbi(buf []byte) error {
		return decodeMethod({{.ContractName}}.Abi.{{.MethodGetter}}["{{.Name}}"], buf, {{.Sig}})
	}`

	methodGetter := "Methods"
	if fnSigResolution {
		methodGetter = "MethodsBySignature"
	}

	inputs := map[string]any{
		"Structs":      res,
		"Sig":          strings.ToLower(string(methodName[0])),
		"Name":         method.Name,
		"ContractName": contractName,
		"TName":        strings.Title(methodName),
		"MethodGetter": methodGetter,
	}

	if fnSigResolution {
		inputs["Name"] = method.Sig()
	}

	renderedString, err := renderTmpl(tmplString, inputs)
	if err != nil {
		return err
	}

	generatedData.resultString = append(generatedData.resultString, renderedString)

	return nil
}

func renderTmpl(tmplStr string, inputs map[string]any) (string, error) {
	tmpl, err := template.New("name").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("failed to load template: %w", err)
	}

	var tpl bytes.Buffer
	if err = tmpl.Execute(&tpl, inputs); err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return tpl.String(), nil
}

/* FINISH ALPHA CHANGE 3768 -- modernize generator interface aliases and replace-all calls -- */
