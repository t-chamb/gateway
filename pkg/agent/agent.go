// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.githedgehog.com/gateway-proto/pkg/dataplane"
	gwintapi "go.githedgehog.com/gateway/api/gwint/v1alpha1"
	"go.githedgehog.com/gateway/api/meta"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/grpclog"
	"google.golang.org/grpc/status"
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

type Service struct {
	cfg      *meta.AgentConfig
	kube     kclient.WithWatch
	curr     *gwintapi.GatewayAgent
	dpConn   *grpc.ClientConn
	dpClient dataplane.ConfigServiceClient
}

func New() *Service {
	return &Service{}
}

func (svc *Service) Run(ctx context.Context) error {
	cfgData, err := os.ReadFile("/etc/hedgehog/gateway-agent/config.yaml")
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}
	svc.cfg = &meta.AgentConfig{}
	if err := kyaml.UnmarshalStrict(cfgData, svc.cfg); err != nil {
		return fmt.Errorf("unmarshalling config file: %w", err)
	}

	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("getting hostname: %w", err)
	}

	if svc.cfg.Name != hostname {
		return fmt.Errorf("agent name %q does not match hostname %q", svc.cfg.Name, hostname) //nolint:err113
	}

	svc.kube, err = newKubeClient(gwintapi.SchemeBuilder)
	if err != nil {
		return fmt.Errorf("creating kube client: %w", err)
	}

	grpclog.SetLoggerV2(NewGRPCLogger(slog.Default(), slog.LevelError))

	svc.dpConn, err = grpc.NewClient(svc.cfg.DataplaneAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),

		// TODO consider using health check https://grpc.io/docs/guides/health-checking/
		// grpc.WithDefaultServiceConfig(`"healthCheckConfig": { "serviceName": "" }`),

		// TODO think about backoff
		// grpc.WithConnectParams(grpc.ConnectParams{
		// 	Backoff: backoff.Config{},
		// }),

		// TODO think about keepalive
		// grpc.WithKeepaliveParams(keepalive.ClientParameters{
		// 	Time:                10 * time.Second, // send pings every 10 seconds if there is no activity
		// 	Timeout:             1 * time.Second,  // wait 1 second for ping ack before considering the connection dead
		// 	PermitWithoutStream: true,             // send pings even without active streams
		// }),
	)
	if err != nil {
		return fmt.Errorf("creating grpc conn: %w", err)
	}
	defer svc.dpConn.Close()

	svc.dpClient = dataplane.NewConfigServiceClient(svc.dpConn)

	retry := false
	for {
		if retry {
			select {
			case <-ctx.Done():
				slog.Warn("Context done, stopping")

				return nil
			case <-time.After(1 * time.Second):
				// Retry watching after a delay
			}

			slog.Info("Retrying to watch agent object")
		}
		retry = true

		if err := svc.watchAgent(ctx); err != nil {
			return fmt.Errorf("watching agent object: %w", err)
		}
	}
}

func (svc *Service) watchAgent(ctx context.Context) error {
	svc.curr = &gwintapi.GatewayAgent{}
	if err := svc.kube.Get(ctx, kclient.ObjectKey{
		Name:      svc.cfg.Name,
		Namespace: svc.cfg.Namespace,
	}, svc.curr); err != nil {
		return fmt.Errorf("getting agent object: %w", err)
	}

	watcher, err := svc.kube.Watch(ctx, &gwintapi.GatewayAgentList{},
		kclient.InNamespace(svc.cfg.Namespace), kclient.MatchingFields{"metadata.name": svc.cfg.Name})
	if err != nil {
		return fmt.Errorf("creating watcher: %w", err)
	}
	defer watcher.Stop()

	enforce := time.NewTicker(5 * time.Second)
	defer enforce.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case event, ok := <-watcher.ResultChan():
			if !ok {
				slog.Warn("Watcher channel closed")

				return nil
			}

			switch event.Type {
			case watch.Added, watch.Modified:
				ag := event.Object.(*gwintapi.GatewayAgent)
				slog.Info("Handling", "name", ag.Name, "ns", ag.Namespace, "uid", ag.UID, "gen", ag.Generation, "curr", svc.curr.Generation, "res", ag.ResourceVersion)

				if ag.Generation == svc.curr.Generation {
					svc.curr = ag

					continue
				}

				if err := svc.enforceDataplaneConfig(ctx, ag); err != nil {
					if status, ok := status.FromError(errors.Unwrap(err)); ok {
						if status.Code() == codes.Unavailable {
							slog.Warn("Dataplane unavailable, will retry", "error", status.Message())

							continue
						}

						slog.Warn("Dataplane error, will retry", "error", status.Message())
					}

					return fmt.Errorf("handling agent: %w", err)
				}

				svc.curr = ag
			case watch.Deleted:
				slog.Warn("Agent object deleted, shutting down")

				return fmt.Errorf("agent object deleted") //nolint:err113
			case watch.Bookmark:
				// Ignore bookmark events
			case watch.Error:
				slog.Warn("Watcher error", "event", event.Type, "object", event.Object)

				return nil
			default:
				slog.Warn("Unknown event type", "event", event.Type, "object", event.Object)

				return nil
			}
		case <-enforce.C:
			if err := svc.enforceDataplaneConfig(ctx, svc.curr); err != nil {
				if status, ok := status.FromError(errors.Unwrap(err)); ok {
					if status.Code() == codes.Unavailable {
						slog.Warn("Dataplane unavailable, will retry", "error", status.Message())

						continue
					}

					slog.Warn("Dataplane error, will retry", "error", status.Message())
				}

				return fmt.Errorf("enforcing config: %w", err)
			}
		}
	}
}

func (svc *Service) enforceDataplaneConfig(ctx context.Context, ag *gwintapi.GatewayAgent) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	resp, err := svc.dpClient.GetConfigGeneration(ctx, &dataplane.GetConfigGenerationRequest{})
	if err != nil {
		return fmt.Errorf("getting config generation: %w", err)
	}
	if resp.Generation != uint64(ag.Generation) { //nolint:gosec // TODO fix proto
		slog.Info("Dataplane config needs to be updated", "current", resp.Generation, "new", ag.Generation)

		gwCfg, err := buildDataplaneConfig(ag)
		if err != nil {
			return fmt.Errorf("building dataplane config: %w", err)
		}

		resp, err := svc.dpClient.UpdateConfig(ctx, &dataplane.UpdateConfigRequest{
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

	// TODO report status to the agent object

	return nil
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
