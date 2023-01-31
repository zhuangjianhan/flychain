// 包编译器包装了 Solidity 和 Vyper 编译器可执行文件 (solc; vyper)。
package compiler

// Contract 包含有关已编译合约的信息，以及它的代码和运行时代码。
type Contract struct {
	Code string `json:"code"`
	RuntimeCode string `json:"runtime-code"`
	Info ContractInfo `json:"info"`
	Hashes map[string]string `json:"hashes"`
}

// ContractInfo 包含有关已编译合约的信息，包括访问
// 到 ABI 定义、源映射、用户和开发人员文档以及元数据。
//
// 取决于源、语言版本、编译器版本和编译器
// options 将提供有关合约如何编译的信息。
type ContractInfo struct {
	Source string `json:"source"`
	Language string `json:"language"`
	LanguageVersion string `json:"languageVersion"`
	CompilerVersion string `json:"compilerVersion"`
	CompilerOptions string `json:"compilerOptions"`
	SrcMap interface{} `json:"srcMap"`
	SrcMapRuntime string `json:"srcMapRuntime"`
	AbiDefinition interface{} `json:"abiDefinition"`
	UserDoc interface{} `json:"userDoc"`
	DeveloperDoc interface{} `json:"developDoc"`
	Metadata string `json:"metadata"`
}