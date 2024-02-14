package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/0xsequence/ethkit/ethartifact"
	"github.com/0xsequence/ethkit/go-ethereum/accounts/abi"
	"github.com/spf13/cobra"
)

func init() {
	werbrpc := &Webrpc{}
	cmd := &cobra.Command{
		Use:   "webrpc",
		Short: "webrpc generator",
		Run:   werbrpc.Run,
	}
	rootCmd.AddCommand(cmd)
}

type Webrpc struct {
}

func (w *Webrpc) Run(cmd *cobra.Command, args []string) {
	artifact, err := ethartifact.ParseArtifactFile("/home/saint/marketplace-api/lib/contracts/artifacts/marketplace/ISequenceMarket.sol/ISequenceMarket.json")
	if err != nil {
		panic(err)
		return
	}
	abilocal, err := abi.JSON(strings.NewReader(string(artifact.ABI)))
	if err != nil {
		panic(err)
	}

	funcMap := template.FuncMap{
		// The name "title" is what the function will be called in the template text.
		"hasStruct":                 hasStruct,
		"goType":                    goType,
		"firstLetterUpper":          firstLetterUpper,
		"argType":                   argType,
		"needArgSeperator":          needArgSeperator,
		"needStructTypeConverstion": needStructTypeConverstion,
		"structTypeConversion":      structTypeConversion,
		"dict":                      dict,
		"isSlice":                   isSlice,
		"webrpcArgType":             webrpcArgType,
	}
	methods := map[string]abi.Method{"createRequestBatch": abilocal.Methods["createRequestBatch"]}

	methods = abilocal.Methods
	files, err := filepath.Glob("./template/*.go.tmpl")
	if err != nil {
		panic(err)
	}
	t, err := template.New("rpc").Funcs(funcMap).ParseFiles(files...)
	if err != nil {
		panic(err)
	}

	err = t.ExecuteTemplate(os.Stdout, "rpc", map[string]interface{}{"Methods": methods, "Structs": getStructTypes(methods), "RpcPackage": "github.com/0xsequence/marketplace-api/proto", "Package": "orderbook"})
	if err != nil {
		panic(err)
	}
}

// hasStruct returns an indicator whether the given type is struct, struct slice
// or struct array.
func hasStruct(t abi.Type) bool {
	switch t.T {
	case abi.SliceTy:
		return hasStruct(*t.Elem)
	case abi.ArrayTy:
		return hasStruct(*t.Elem)
	case abi.TupleTy:
		return true
	default:
		return false
	}
}

func isSlice(t abi.Type) bool {
	switch t.T {
	case abi.SliceTy:
		return true
	case abi.AddressTy:
		return true
	default:
		return false
	}
}

func goType(ethType string) string {
	switch ethType {
	case "uint256":
		return "*big.Int"
	case "uint96":
		return "*big.Int"
	case "address":
		return "common.Address"
	case "address[]":
		return "[]common.Address"
	case "uint256[]":
		return "[]*big.Int"
	case "bool":
		return "bool"
	default:
		panic(fmt.Sprintf("goType is not defined for eth type %s", ethType))
	}
}

func needStructTypeConverstion(ethType string) bool {
	switch ethType {
	case "uint256":
		return true
	case "uint96":
		return true
	case "address":
		return true
	case "bool":
		return false
	default:
		panic(fmt.Sprintf("type conversion not defined for struct %s", ethType))
	}
}

func structTypeConversion(ethType string) string {
	switch ethType {
	case "uint256":
		return ".Int()"
	case "uint96":
		return ".Int()"
	case "address":
		return ".ToAddress()"
	default:
		return ""
	}
}

func argType(arg abi.Type) string {
	if hasStruct(arg) {
		switch arg.T {
		case abi.SliceTy:
			return "[]" + argType(*arg.Elem)
		case abi.ArrayTy:
			return "[]" + argType(*arg.Elem)
		default:
			return "*proto." + arg.TupleRawName
		}
	}
	switch arg.String() {
	case "address":
		return "string"
	case "address[]":
		return "[]string"
	case "uint256":
		return "string"
	case "uint256[]":
		return "[]string"
	default:
		panic(fmt.Sprintf("arg type not defined for %s", arg.String()))
	}

}

func webrpcArgType(arg abi.Type) string {
	if hasStruct(arg) {
		switch arg.T {
		case abi.SliceTy:
			return "[]" + webrpcArgType(*arg.Elem)
		case abi.ArrayTy:
			return "[]" + webrpcArgType(*arg.Elem)
		default:
			return arg.TupleRawName
		}
	}
	switch arg.String() {
	case "address":
		return "string"
	case "address[]":
		return "[]string"
	case "uint256":
		return "string"
	case "uint256[]":
		return "[]string"
	default:
		panic(fmt.Sprintf("arg type not defined for %s", arg.String()))
	}
}

func getStructTypes(methods map[string]abi.Method) []abi.Type {
	unique := map[string]interface{}{}
	structTypes := []abi.Type{}
	for _, method := range methods {
		for _, input := range method.Inputs {
			if !hasStruct(input.Type) {
				continue
			}
			s := getStruct(input.Type)
			_, ok := unique[s.TupleRawName]
			if ok {
				continue
			}
			unique[s.TupleRawName] = true
			structTypes = append(structTypes, s)
		}
	}
	return structTypes
}

func getStruct(t abi.Type) abi.Type {
	switch t.T {
	case abi.TupleTy:
		return t
	case abi.SliceTy:
		return getStruct(*t.Elem)
	case abi.AddressTy:
		return getStruct(*t.Elem)
	default:
		panic("failed to get struct")
	}
}

func firstLetterUpper(in string) string {
	return strings.ToUpper(in[:1]) + in[1:]
}

func needArgSeperator(argIndex int, noOfArg int) bool {
	return argIndex < noOfArg-1
}

func dict(values ...interface{}) (map[string]interface{}, error) {
	if len(values)%2 != 0 {
		return nil, errors.New("invalid dict call")
	}
	dict := make(map[string]interface{}, len(values)/2)
	for i := 0; i < len(values); i += 2 {
		key, ok := values[i].(string)
		if !ok {
			return nil, errors.New("dict keys must be strings")
		}
		dict[key] = values[i+1]
	}
	return dict, nil
}
