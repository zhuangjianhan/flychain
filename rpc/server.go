package rpc

import (
	"context"
	"flychain/log"
	"io"
	"sync"
	"sync/atomic"
)

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

// Server is an RPC server
type Server struct {
	services serviceRegistry
	idgen    func() ID

	mutex sync.Mutex
	codecs map[ServerCodec]struct{}
	run   int32
}

// NewServer 创建一个没有注册处理程序的新服务器实例。
func NewServer() *Server {
	server := &Server{
		idgen: randomIDGenerator(),
		codecs: make(map[ServerCodec]struct{}),
		run:   1,
	}
	// 注册默认服务，提供有关 RPC 服务的元信息，例如
	// 作为它提供的服务和方法。
	rpcService := &RPCService{server}
	server.RegisterName(MetadataApi, rpcService)
	return server
}

// RegisterName 在给定名称下为给定接收器类型创建服务。当没有
// 给定接收器上的方法匹配标准是 RPC 方法或
// 订阅返回一个错误。否则，将创建一个新服务并将其添加到
// 此服务器提供给客户端的服务集合。
func (s *Server) RegisterName(name string, receiver interface{}) error {
	return s.services.registerName(name, receiver)
}

// ServeCodec 从编解码器读取传入请求，调用适当的回调并写入
// 使用给定的编解码器返回响应。它将阻塞直到编解码器关闭或
// 服务器已停止。在任何一种情况下，编解码器都是关闭的。
//
// 请注意，不再支持编解码器选项。
func (s *Server) ServerCodec(codec ServerCodec, options CodecOption) {
	//defer codec.close()
}

func (s *Server) trackCodec(codec ServerCodec) bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if atomic.LoadInt32(&s.run) == 0 {
		return false // 如果服务器停止，则不提供服务。
	}
	s.codecs[codec] = struct{}{}
	return true
}

func (s *Server) untrackCodec(codec ServerCodec) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	delete(s.codecs, codec)
}

// serveSingleRequest 从给定的编解码器读取并处理单个 RPC 请求。这
// 用于服务 HTTP 连接。不允许订阅和反向调用
// 这种模式。
func (s *Server) serveSingleRequest(ctx context.Context, codec ServerCodec) {
	// Don't serve if server is stopped.
	if atomic.LoadInt32(&s.run) == 0 {
		return
	}

	h := NewHandler(ctx, codec, s.idgen, &s.services)
	h.allowSubscribe = false
	defer h.close(io.EOF, nil)

	reqs, batch, err := codec.readBatch()
	if err != nil {
		if err != io.EOF {
			resp := errorMessage(&invalidMessageError{"parse error"})
			codec.writeJSON(ctx, resp, true)
		}
		return
	}
	if batch {
		h.handleBatch(reqs)
	} else {
		h.handleMsg(reqs[0])
	}
}

// Stop 停止读取新的请求，等待 stopPendingRequestTimeout 允许挂起
// 请求完成，然后关闭所有将取消挂起请求的编解码器和
// 订阅。
func (s *Server) Stop() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if atomic.CompareAndSwapInt32(&s.run, 1, 0) {
		log.Debug("RPC server shutting down")
		for codec := range s.codecs {
			codec.close()
		}
	}
}

// RPCService 提供有关服务器的元信息。
// 例如提供有关已加载模块的信息。
type RPCService struct {
	server *Server
}

// 模块返回 RPC 服务列表及其版本号
func (s *RPCService) Modules() map[string]string {
	s.server.services.mu.Lock()
	defer s.server.services.mu.Unlock()

	modules := make(map[string]string)
	for name := range s.server.services.services {
		modules[name] = "1.0"
	}
	return modules
}