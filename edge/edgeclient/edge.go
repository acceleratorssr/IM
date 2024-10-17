// Code generated by goctl. DO NOT EDIT.
// Source: edge.proto

package edgeclient

import (
	"context"

	"IM/edge/edge"

	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

type (
	Request  = edge.Request
	Response = edge.Response

	Edge interface {
		Ping(ctx context.Context, in *Request, opts ...grpc.CallOption) (*Response, error)
	}

	defaultEdge struct {
		cli zrpc.Client
	}
)

func NewEdge(cli zrpc.Client) Edge {
	return &defaultEdge{
		cli: cli,
	}
}

func (m *defaultEdge) Ping(ctx context.Context, in *Request, opts ...grpc.CallOption) (*Response, error) {
	client := edge.NewEdgeClient(m.cli.Conn())
	return client.Ping(ctx, in, opts...)
}
