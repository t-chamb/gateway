// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
	"fmt"
	"log/slog"

	"go.githedgehog.com/gateway-proto/pkg/protoyaml"
)

type MockConfigServiceServer struct {
	UnimplementedConfigServiceServer
	cfg *GatewayConfig
	log bool
}

func NewMockConfigServiceServer(log bool) *MockConfigServiceServer {
	return &MockConfigServiceServer{
		cfg: &GatewayConfig{
			Generation: 0,
		},
		log: log,
	}
}

func (m *MockConfigServiceServer) GetObservedConfig() *GatewayConfig {
	return m.cfg
}

var _ ConfigServiceServer = &MockConfigServiceServer{}

func (m *MockConfigServiceServer) GetConfig(context.Context, *GetConfigRequest) (*GatewayConfig, error) {
	if m.log {
		slog.Info("GetConfig called", "gen", m.cfg.Generation)
	}

	return m.cfg, nil
}

func (m *MockConfigServiceServer) GetConfigGeneration(context.Context, *GetConfigGenerationRequest) (*GetConfigGenerationResponse, error) {
	if m.log {
		slog.Info("GetConfigGeneration called", "gen", m.cfg.Generation)
	}

	return &GetConfigGenerationResponse{
		Generation: m.cfg.Generation,
	}, nil
}

func (m *MockConfigServiceServer) UpdateConfig(_ context.Context, req *UpdateConfigRequest) (*UpdateConfigResponse, error) {
	if m.log {
		slog.Info("UpdateConfig called", "gen", m.cfg.Generation)

		data, err := protoyaml.MarshalYAML(req.Config)
		if err != nil {
			slog.Warn("failed to marshal config", "err", err)
		} else {
			fmt.Println("---")
			fmt.Println(string(data))
		}
	}

	m.cfg = req.Config

	return &UpdateConfigResponse{
		Error: Error_ERROR_NONE,
	}, nil
}
