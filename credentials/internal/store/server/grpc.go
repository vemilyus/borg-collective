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

package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/vemilyus/borg-collective/credentials/internal/proto"
	"github.com/vemilyus/borg-collective/credentials/internal/store/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type credStoreServer struct {
	proto.UnimplementedCredStoreServer
	state *service.State
}

func (serv credStoreServer) GetInfo(_ context.Context, _ *proto.Unit) (*proto.StoreInfo, error) {
	return serv.state.StoreInfo(), nil
}

func (serv credStoreServer) UnlockVault(_ context.Context, credentials *proto.AdminCredentials) (*proto.Unit, error) {
	if err := serv.state.Unlock(credentials); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &proto.Unit{}, nil
}

func (serv credStoreServer) LockVault(_ context.Context, _ *proto.Unit) (*proto.Unit, error) {
	ok := serv.state.Lock()
	if !ok {
		log.Debug().Msg("failed to lock vault")
	}

	return &proto.Unit{}, nil
}

func (serv credStoreServer) SetRecoveryRecipient(_ context.Context, recipient *proto.RecoveryRecipient) (*proto.Unit, error) {
	if err := serv.state.SetRecoveryRecipient(recipient); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &proto.Unit{}, nil
}

func (serv credStoreServer) CreateVaultItem(_ context.Context, creation *proto.ItemCreation) (*proto.Item, error) {
	item, err := serv.state.CreateVaultItem(creation)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return item, nil
}

func (serv credStoreServer) ListVaultItems(search *proto.ItemSearch, itemStream grpc.ServerStreamingServer[proto.Item]) error {
	items, err := serv.state.ListVaultItems(search)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	for _, item := range items {
		err = itemStream.Send(
			&proto.Item{
				Id:          item.Id.String(),
				Description: item.Description,
				Checksum:    item.Checksum,
				CreatedAt:   item.ModifiedAt.UnixMilli(),
			},
		)

		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}

	return nil
}

func (serv credStoreServer) DeleteVaultItems(deletion *proto.ItemDeletion, itemStream grpc.ServerStreamingServer[proto.Item]) error {
	deletedIds, err := serv.state.DeleteVaultItems(deletion)
	if err != nil {
		return status.Error(codes.Internal, err.Error())
	}

	for _, id := range deletedIds {
		err = itemStream.Send(&proto.Item{
			Id: id.String(),
		})

		if err != nil {
			return status.Error(codes.Internal, err.Error())
		}
	}

	return nil
}

func (serv credStoreServer) ReadVaultItem(ctx context.Context, request *proto.ItemRequest) (*proto.ItemValue, error) {
	p, _ := peer.FromContext(ctx)
	itemValue, err := serv.state.ReadVaultItem(request, p)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return itemValue, nil
}

func (serv credStoreServer) CreateClientCredentials(_ context.Context, creation *proto.ClientCreation) (*proto.ClientCredentials, error) {
	credentials, err := serv.state.CreateClientCredentials(creation)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return credentials, nil
}

func NewGrpcServer(state *service.State) *grpc.Server {
	logger := log.Logger

	var loggingOpts []logging.Option
	loggingOpts = append(loggingOpts, logging.WithLogOnEvents(logging.StartCall, logging.FinishCall))

	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(proto.ZeroInterceptor()),
		grpc.ChainStreamInterceptor(proto.ZeroStreamInterceptor()),
		grpc.ChainUnaryInterceptor(logging.UnaryServerInterceptor(interceptorLogger(logger), loggingOpts...)),
		grpc.ChainStreamInterceptor(logging.StreamServerInterceptor(interceptorLogger(logger), loggingOpts...)),
	)

	proto.RegisterCredStoreServer(
		grpcServer,
		credStoreServer{
			UnimplementedCredStoreServer: proto.UnimplementedCredStoreServer{},
			state:                        state,
		},
	)

	grpc_prometheus.Register(grpcServer)

	return grpcServer
}

func interceptorLogger(l zerolog.Logger) logging.Logger {
	return logging.LoggerFunc(func(ctx context.Context, level logging.Level, msg string, fields ...any) {
		l := l.With().Fields(fields).Logger()

		switch level {
		case logging.LevelDebug:
			l.Trace().Msg(msg)
		case logging.LevelInfo:
			l.Debug().Msg(msg)
		case logging.LevelWarn:
			l.Warn().Msg(msg)
		case logging.LevelError:
			l.Error().Msg(msg)
		default:
			l.Warn().Err(errors.New(fmt.Sprintf("unknown level: %v", level))).Msg(msg)
		}
	})
}
