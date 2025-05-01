// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/samber/lo"
	"go.githedgehog.com/gateway-proto/pkg/dataplane"
	gwintapi "go.githedgehog.com/gateway/api/gwint/v1alpha1"
	"go.githedgehog.com/gateway/api/meta"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
	kyaml "sigs.k8s.io/yaml"
)

const (
	ConfigDir  = "/etc/hedgehog/gateway-agent"
	ConfigFile = "config.yaml"
)

func Run(ctx context.Context) error {
	cfgData, err := os.ReadFile("/etc/hedgehog/gateway-agent/config.yaml")
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}
	cfg := &meta.AgentConfig{}
	if err := kyaml.UnmarshalStrict(cfgData, cfg); err != nil {
		return fmt.Errorf("unmarshalling config file: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("getting hostname: %w", err)
	}

	if cfg.Name != hostname {
		return fmt.Errorf("agent name %q does not match hostname %q", cfg.Name, hostname) //nolint:err113
	}

	kube, err := newKubeClient(gwintapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	retry := false
	for {
		if retry {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
				// Retry after a delay
			}

			slog.Info("Retrying to watch agent object")
		}
		retry = true

		watcher, err := kube.Watch(ctx, &gwintapi.GatewayAgentList{},
			kclient.InNamespace(cfg.Namespace), kclient.MatchingFields{"metadata.name": cfg.Name})
		if err != nil {
			return fmt.Errorf("creating watcher: %w", err)
		}
		defer watcher.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("Context done, stopping")

				return nil
			case event, ok := <-watcher.ResultChan():
				if !ok {
					slog.Warn("Watcher channel closed")

					continue
				}

				switch event.Type {
				case watch.Added, watch.Modified:
					// TODO store previous state?
					if err := handleAgent(ctx, event.Object.(*gwintapi.GatewayAgent)); err != nil {
						slog.Error("Error handling agent", "error", err)

						return fmt.Errorf("handling agent: %w", err)
					}
				case watch.Deleted:
					slog.Warn("Agent object deleted, shutting down")

					return fmt.Errorf("agent object deleted") //nolint:err113
				case watch.Bookmark:
					// Ignore bookmark events
				case watch.Error:
					slog.Warn("Watcher error", "event", event.Type, "object", event.Object)

					continue
				default:
					slog.Warn("Unknown event type", "event", event.Type, "object", event.Object)

					continue
				}
			}
		}
	}
}

func newKubeClient(schemeBuilders ...*scheme.Builder) (kclient.WithWatch, error) {
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

func handleAgent(_ context.Context, ag *gwintapi.GatewayAgent) error {
	slog.Info("Gateway agent loaded", "name", ag.Name, "ns", ag.Namespace, "uid", ag.UID, "gen", ag.Generation, "res", ag.ResourceVersion)

	for vpcName, vpc := range ag.Spec.VPCs {
		slog.Info("VPC", "name", vpcName, "internalID", vpc.InternalID)
	}

	for peeringName, peering := range ag.Spec.Peerings {
		slog.Info("Peering", "name", peeringName, "vpcs", strings.Join(lo.Keys(peering.Peering), ","))
	}

	// TODO replace with real client
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
