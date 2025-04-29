// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/samber/lo"
	"go.githedgehog.com/gateway-proto/pkg/dataplane"
	gwintapi "go.githedgehog.com/gateway/api/gwint/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

func Run(ctx context.Context) error {
	// TODO pass expected host in config and match just in case
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("getting hostname: %w", err)
	}

	kube, err := newKubeClient(gwintapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	ag := &gwintapi.GatewayAgent{}
	if err := kube.Get(ctx, kclient.ObjectKey{
		Name:      hostname,
		Namespace: kmetav1.NamespaceDefault, // TODO
	}, ag); err != nil {
		return fmt.Errorf("getting agent: %w", err)
	}

	slog.Info("Gateway agent object loaded", "name", ag.Name, "namespace", ag.Namespace, "uid", ag.UID, "gen", ag.Generation)

	for vpcName, vpc := range ag.Spec.VPCs {
		slog.Info("VPC", "name", vpcName, "internalID", vpc.InternalID)
	}

	for peeringName, peering := range ag.Spec.Peerings {
		slog.Info("Peering", "name", peeringName, "vpcs", strings.Join(lo.Keys(peering.Peering), ","))
	}

	// TODO replace with real client
	if dpClient := dataplane.NewMockConfigServiceServer(); dpClient == nil {
		return fmt.Errorf("creating mock dataplane client") //nolint:err113
	}

	conn, err := grpc.NewClient("127.0.0.1:51234",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("creating grpc conn: %w", err)
	}
	defer conn.Close()

	client := dataplane.NewConfigServiceClient(conn)
	_ = client

	return nil
}

func newKubeClient(schemeBuilders ...*scheme.Builder) (kclient.Client, error) {
	cfg, err := kctrl.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("getting kubeconfig using default path or in-cluster config: %w", err)
	}

	scheme := runtime.NewScheme()
	for _, schemeBuilder := range schemeBuilders {
		if err := schemeBuilder.AddToScheme(scheme); err != nil {
			return nil, fmt.Errorf("adding scheme %s to runtime: %w", schemeBuilder.GroupVersion.String(), err)
		}
	}

	kubeClient, err := kclient.NewWithWatch(cfg, kclient.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("creating kube client: %w", err)
	}

	return kubeClient, nil
}
