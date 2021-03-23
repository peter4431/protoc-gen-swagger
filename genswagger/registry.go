package genswagger

import (
	"github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway/descriptor"
)

type SRegistry struct {
	*descriptor.Registry
	wrapRespCode bool
	YAPIUrl      string // yapi 服务器地址
	YAPISchema   string // yapi schema
	YAPIToken    string // yapi token
	YAPIMerge    string // yapi 合并方式 normal-新增不覆盖, good-智能合并, merge-完全覆盖
}

func NewRegistry() *SRegistry {
	ret := &SRegistry{
		Registry:     descriptor.NewRegistry(),
		wrapRespCode: false,
	}
	return ret
}

func (sr *SRegistry) SetWrapRespCode(v bool) {
	sr.wrapRespCode = v
}

func (sr *SRegistry) GetWrapRespCode() bool {
	return sr.wrapRespCode
}
