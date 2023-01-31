package compiler

import (
	"encoding/json"
	"fmt"
)

// --组合输出格式
type solcOutput struct {
	Contracts map[string]struct {
		BinRuntime                                  string `json:"bin-runtime"`
		SrcMapRuntime                               string `json:"srcmap-runtime"`
		Bin, SrcMap, Abi, Devdoc, Userdoc, Metadata string
		Hashes                                      map[string]string
	}
	Version string
}

// solidity v.0.8 改变了 ABI、Devdoc 和 Userdoc 的序列化方式
type solcOutputV8 struct {
	Contracts map[string]struct {
		BinRuntime            string `json:"bin-runtime"`
		SrcMapRuntime         string `json:"srcmap-runtime"`
		Bin, SrcMap, Metadata string
		Abi                   interface{}
		Devdoc                interface{}
		UserDoc               interface{}
		Hashes                map[string]string
	}
	Version string
}

// ParseCombinedJSON 采用 solc --combined-output 运行的直接输出和
// 将其解析为字符串合约名称到合约结构的映射。这
// 提供的源码、语言和编译器版本，编译器选项都是
// 传递到合约结构中。
//
// solc 输出应包含 ABI、源映射、用户文档和开发文档。
//
// 如果 JSON 格式错误或缺少数据，或者如果 JSON
// 嵌入在 JSON 中的格式不正确。
func ParseCombinedJSON(combinedJSON []byte, source string, languageVersion string, compilerVersion string, compilerOptions string) (map[string]*Contract, error) {
	var output solcOutput
	if err := json.Unmarshal(combinedJSON, &output); err != nil {
		// 尝试使用新的 solidity v.0.8.0 规则解析输出
		return ParseCombinedJSONV8(combinedJSON, source, languageVersion, compilerVersion, compilerOptions)
	}
	// 编译成功，组装并返回合约。
	contracts := make(map[string]*Contract)
	for name, info := range output.Contracts {
		// 解析单独的编译结果。
		var abi, userdoc, devdoc interface{}
		if err := json.Unmarshal([]byte(info.Abi), &abi); err != nil {
			return nil, fmt.Errorf("solc: error reading abi definition (%v)", err)
		}
		if err := json.Unmarshal([]byte(info.Userdoc), &userdoc); err != nil {
			return nil, fmt.Errorf("solc: error reading userdoc definition (%v)", err)
		}
		if err := json.Unmarshal([]byte(info.Devdoc), &devdoc); err != nil {
			return nil, fmt.Errorf("solc: error reading devdoc definition (%v)", err)
		}

		contracts[name] = &Contract{
			Code: "0x" + info.Bin,
			RuntimeCode: "0x" + info.BinRuntime,
			Hashes: info.Hashes,
			Info: ContractInfo{
				Source: source,
				Language: "Solidity",
				LanguageVersion: languageVersion,
				CompilerVersion: compilerVersion,
				CompilerOptions: compilerOptions,
				SrcMap: info.SrcMap,
				SrcMapRuntime: info.SrcMapRuntime,
				AbiDefinition: abi,
				UserDoc: userdoc,
				DeveloperDoc: devdoc,
				Metadata: info.Metadata,
			},
		}
	}
	return contracts, nil
}

// parseCombinedJSONV8解析solc的直接输出--combined-output
// 并使用 solidity v.0.8.0 及更高版本的规则对其进行解析。
func ParseCombinedJSONV8(combinedJSON []byte, source string, languageVersion string, compilerVersion string, compilerOptions string) (map[string]*Contract, error) {
	var output solcOutputV8
	if err := json.Unmarshal(combinedJSON, &output); err != nil {
		return nil, err
	}
	// 编译成功，组装并返回合约。
	contracts := make(map[string]*Contract)
	for name, info := range output.Contracts {
		contracts[name] = &Contract{
			Code: "0x" + info.Bin,
			RuntimeCode: "0x" + info.BinRuntime,
			Hashes: info.Hashes,
			Info: ContractInfo{
				Source: source,
				Language: "Solidity",
				LanguageVersion: languageVersion,
				CompilerVersion: compilerVersion,
				CompilerOptions: compilerOptions,
				SrcMap: info.SrcMap,
				SrcMapRuntime: info.SrcMapRuntime,
				AbiDefinition: info.Abi,
				UserDoc: info.UserDoc,
				DeveloperDoc: info.Devdoc,
				Metadata: info.Metadata,
			},
		}
	}
	return contracts, nil
}
