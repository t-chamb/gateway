// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package dataplane

import (
	"context"
)

type MockConfigServiceServer struct {
	UnimplementedConfigServiceServer
	cfg *GatewayConfig
}

func NewMockConfigServiceServer() *MockConfigServiceServer {
	return &MockConfigServiceServer{
		cfg: &GatewayConfig{
			Generation: 0,
		},
	}
}

var _ ConfigServiceServer = &MockConfigServiceServer{}

func (m *MockConfigServiceServer) GetConfig(context.Context, *GetConfigRequest) (*GatewayConfig, error) {
	return m.cfg, nil
}

func (m *MockConfigServiceServer) GetConfigGeneration(context.Context, *GetConfigGenerationRequest) (*GetConfigGenerationResponse, error) {
	gen := uint64(0)
	if m.cfg != nil {
		gen = m.cfg.Generation
	}

	return &GetConfigGenerationResponse{
		Generation: gen,
	}, nil
}

func (m *MockConfigServiceServer) UpdateConfig(_ context.Context, req *UpdateConfigRequest) (*UpdateConfigResponse, error) {
	m.cfg = req.Config

	return &UpdateConfigResponse{
		Error: Error_ERROR_NONE,
	}, nil
}

func (m *MockConfigServiceServer) GetObservedConfig() *GatewayConfig {
	return m.cfg
}
