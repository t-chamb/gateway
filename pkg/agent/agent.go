// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
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
					if err := handleAgent(ctx, cfg, event.Object.(*gwintapi.GatewayAgent)); err != nil {
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

func handleAgent(ctx context.Context, cfg *meta.AgentConfig, ag *gwintapi.GatewayAgent) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	slog.Info("Gateway agent loaded", "name", ag.Name, "ns", ag.Namespace, "uid", ag.UID, "gen", ag.Generation, "res", ag.ResourceVersion)

	for vpcName, vpc := range ag.Spec.VPCs {
		slog.Info("VPC", "name", vpcName, "internalID", vpc.InternalID)
	}

	for peeringName, peering := range ag.Spec.Peerings {
		slog.Info("Peering", "name", peeringName, "vpcs", strings.Join(lo.Keys(peering.Peering), ","))
	}

	// TODO probably run this in a separate goroutine to keep connection open and react to reconnects
	conn, err := grpc.NewClient(cfg.DataplaneAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("creating grpc conn: %w", err)
	}
	defer conn.Close()

	client := dataplane.NewConfigServiceClient(conn)

	{
		resp, err := client.GetConfigGeneration(ctx, &dataplane.GetConfigGenerationRequest{})
		if err != nil {
			return fmt.Errorf("getting config generation: %w", err)
		}
		if resp.Generation != uint64(ag.Generation) { //nolint:gosec // TODO fix proto
			slog.Info("Dataplane config needs to be updated", "current", resp.Generation, "new", ag.Generation)

			gwCfg, err := buildDataplaneConfig(ag)
			if err != nil {
				return fmt.Errorf("building dataplane config: %w", err)
			}

			resp, err := client.UpdateConfig(ctx, &dataplane.UpdateConfigRequest{
				Config: gwCfg,
			})
			if err != nil {
				return fmt.Errorf("updating config: %w", err)
			}

			slog.Info("Dataplane config updated", "gen", ag.Generation, "message", resp.Message, "error", resp.Error)

			if resp.Error != dataplane.Error_ERROR_NONE {
				return fmt.Errorf("updating config returned error: %s", resp.Error.String()) //nolint:goerr113
			}
		}
	}

	return nil
}

func buildDataplaneConfig(ag *gwintapi.GatewayAgent) (*dataplane.GatewayConfig, error) {
	cfg := &dataplane.GatewayConfig{
		Generation: uint64(ag.Generation), //nolint:gosec // TODO fix proto
		Device: &dataplane.Device{
			Driver:   dataplane.PacketDriver_KERNEL,
			Hostname: ag.Name,
			Loglevel: dataplane.LogLevel_DEBUG,
		},
		Underlay: &dataplane.Underlay{ // TODO replace with actual generated config
			Vrf: []*dataplane.VRF{
				{
					Name: "default",
					Interfaces: []*dataplane.Interface{
						{
							Name:   "eth0",
							Ipaddr: "10.0.0.1/32",
							Type:   dataplane.IfType_IF_TYPE_ETHERNET,
							Role:   dataplane.IfRole_IF_ROLE_FABRIC,
						},
						{
							Name:   "eth1",
							Ipaddr: "10.0.0.1/32",
							Type:   dataplane.IfType_IF_TYPE_ETHERNET,
							Role:   dataplane.IfRole_IF_ROLE_EXTERNAL,
						},
					},
				},
			},
		},
		Overlay: &dataplane.Overlay{},
	}

	// TODO do we need to bother with sorting?

	for _, vpcName := range slices.Sorted(maps.Keys(ag.Spec.VPCs)) {
		vpc := ag.Spec.VPCs[vpcName]
		cfg.Overlay.Vpcs = append(cfg.Overlay.Vpcs, &dataplane.VPC{
			Name: vpcName,
			Id:   vpc.InternalID,
			Vni:  vpc.VNI,
		})
	}

	for _, peeringName := range slices.Sorted(maps.Keys(ag.Spec.Peerings)) {
		peering := ag.Spec.Peerings[peeringName]
		p := &dataplane.VpcPeering{
			Name: peeringName,
			For:  []*dataplane.PeeringEntryFor{},
		}

		for _, vpcName := range slices.Sorted(maps.Keys(peering.Peering)) {
			vpc := peering.Peering[vpcName]
			exposes := []*dataplane.Expose{}

			for _, expose := range vpc.Expose {
				ips := []*dataplane.PeeringIPs{}
				as := []*dataplane.PeeringAs{}

				for _, ipEntry := range expose.IPs {
					// TODO validate
					switch {
					case ipEntry.CIDR != "":
						ips = append(ips, &dataplane.PeeringIPs{
							Rule: &dataplane.PeeringIPs_Cidr{Cidr: ipEntry.CIDR},
						})
					case ipEntry.Not != "":
						ips = append(ips, &dataplane.PeeringIPs{
							Rule: &dataplane.PeeringIPs_Not{Not: ipEntry.Not},
						})
					case ipEntry.VPCSubnet != "":
						return nil, fmt.Errorf("vpcSubnet not supported yet") //nolint:goerr113 // TODO
					default:
						return nil, fmt.Errorf("invalid IP entry in peering %s / vpc %s: %v", peeringName, vpcName, ipEntry) //nolint:goerr113
					}
				}

				for _, asEntry := range expose.As {
					// TODO validate
					switch {
					case asEntry.CIDR != "":
						as = append(as, &dataplane.PeeringAs{
							Rule: &dataplane.PeeringAs_Cidr{Cidr: asEntry.CIDR},
						})
					case asEntry.Not != "":
						as = append(as, &dataplane.PeeringAs{
							Rule: &dataplane.PeeringAs_Not{Not: asEntry.Not},
						})
					default:
						return nil, fmt.Errorf("invalid IP entry in peering %s / vpc %s: %v", peeringName, vpcName, asEntry) //nolint:goerr113
					}
				}

				exposes = append(exposes, &dataplane.Expose{
					Ips: ips,
					As:  as,
				})
			}

			p.For = append(p.For, &dataplane.PeeringEntryFor{
				Vpc:    vpcName,
				Expose: exposes,
			})
		}

		cfg.Overlay.Peerings = append(cfg.Overlay.Peerings, p)
	}

	return cfg, nil
}
