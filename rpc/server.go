package rpc

const MetadataApi = "rpc"
const EngineApi = "engine"

// CodecOption 指定编解码器支持的消息类型。
//
// 已弃用：服务器不再支持此选项。
type CodecOption int

const (
	// OptionMethodInvocation 表示编解码器支持RPC方法调用
	OptionMethodInvocation CodecOption = 1 << iota

	// OptionSubscriptions 表示编解码器支持 RPC 通知
	OptionSubscriptions = 1 << iota // 支持发布订阅
)

//Server is an RPC server
type Server struct {
	services 
	
}

