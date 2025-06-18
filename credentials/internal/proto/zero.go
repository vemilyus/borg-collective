// Copyright (C) 2025 Alex Katlein
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

package proto

import (
	"context"
	"runtime"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type Zero interface {
	PbZero()
}

func ZeroInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		defer func() {
			if reqZero, ok := req.(Zero); ok {
				reqZero.PbZero()
			}
		}()

		resp, err := handler(ctx, req)
		defer func(resp any) {
			if resp != nil {
				runtime.AddCleanup(&req, func(s interface{}) {
					if respZero, ok := s.(Zero); ok {
						respZero.PbZero()
					}
				}, resp)
			}
		}(resp)

		if err != nil {
			return nil, err
		}

		return resp, nil
	}
}

type zeroServerStreamWrapper struct {
	delegate grpc.ServerStream
}

func (z *zeroServerStreamWrapper) SetHeader(md metadata.MD) error {
	return z.delegate.SetHeader(md)
}

func (z *zeroServerStreamWrapper) SendHeader(md metadata.MD) error {
	return z.delegate.SendHeader(md)
}

func (z *zeroServerStreamWrapper) SetTrailer(md metadata.MD) {
	z.delegate.SetTrailer(md)
}

func (z *zeroServerStreamWrapper) Context() context.Context {
	return z.delegate.Context()
}

func (z *zeroServerStreamWrapper) SendMsg(m any) error {
	runtime.AddCleanup(&z.delegate, func(s any) {
		if mZero, ok := m.(Zero); ok {
			mZero.PbZero()
		}
	}, m)

	return z.delegate.SendMsg(m)
}

func (z *zeroServerStreamWrapper) RecvMsg(m any) error {
	runtime.AddCleanup(&z.delegate, func(s any) {
		if mZero, ok := m.(Zero); ok {
			mZero.PbZero()
		}
	}, m)

	return z.delegate.RecvMsg(m)
}

func ZeroStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		zeroStream := &zeroServerStreamWrapper{
			delegate: ss,
		}

		return handler(srv, zeroStream)
	}
}
