// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"crypto/tls"
	"fmt"
	"log/slog"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/go-logr/logr"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/klog/v2"
	kctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	kyaml "sigs.k8s.io/yaml"

	"go.githedgehog.com/gateway/pkg/ctrl"
	"go.githedgehog.com/gateway/pkg/version"

	helmapi "github.com/k3s-io/helm-controller/pkg/apis/helm.cattle.io/v1"
	gatewayv1alpha1 "go.githedgehog.com/gateway/api/gateway/v1alpha1"
	gwintv1alpha1 "go.githedgehog.com/gateway/api/gwint/v1alpha1"
	"go.githedgehog.com/gateway/api/meta"
	// +kubebuilder:scaffold:imports
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(gatewayv1alpha1.AddToScheme(scheme))
	utilruntime.Must(gwintv1alpha1.AddToScheme(scheme))
	utilruntime.Must(helmapi.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	// TODO make it configurable
	logLevel := slog.LevelDebug

	logW := os.Stderr
	handler := tint.NewHandler(logW, &tint.Options{
		Level:      logLevel,
		TimeFormat: time.StampMilli,
		NoColor:    !isatty.IsTerminal(logW.Fd()),
	})

	logger := slog.New(handler)
	slog.SetDefault(logger)
	kctrl.SetLogger(logr.FromSlogHandler(handler))
	klog.SetSlogLogger(logger)

	if err := run(); err != nil {
		slog.Error("Failed to run", "error", err)
		os.Exit(1)
	}
}

func run() error {
	slog.Info("Starting gateway-ctrl", "version", version.Version)

	cfgData, err := os.ReadFile("/etc/hedgehog/gateway-ctrl/config.yaml")
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}
	cfg := &meta.GatewayCtrlConfig{}
	if err := kyaml.Unmarshal(cfgData, cfg); err != nil {
		return fmt.Errorf("unmarshalling config file: %w", err)
	}

	// Disabling http/2 will prevent from being vulnerable to the HTTP/2 Stream Cancellation and Rapid Reset CVEs.
	// For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	tlsOpts := []func(*tls.Config){
		func(c *tls.Config) {
			slog.Info("Disabling http/2")
			c.NextProtos = []string{"http/1.1"}
		},
	}

	// TODO: enable secure metrics
	// FilterProvider is used to protect the metrics endpoint with authn/authz.
	// These configurations ensure that only authorized users and service accounts
	// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
	// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
	// metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization

	mgr, err := kctrl.NewManager(kctrl.GetConfigOrDie(), kctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: ":8080",
			TLSOpts:     tlsOpts,
		},
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    9443,
			TLSOpts: tlsOpts,
		}),
		HealthProbeBindAddress: ":8081",
		LeaderElection:         true,
		LeaderElectionID:       "gateway.githedgehog.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		return fmt.Errorf("creating manager: %w", err)
	}

	// Controllers
	if err := ctrl.SetupGatewayReconcilerWith(mgr, cfg); err != nil {
		return fmt.Errorf("setting up gateway controller: %w", err)
	}
	if err := ctrl.SetupVPCInfoReconcilerWith(mgr); err != nil {
		return fmt.Errorf("setting up vpcinfo controller: %w", err)
	}

	// Webhooks
	if err := ctrl.SetupGatewayWebhookWith(mgr); err != nil {
		return fmt.Errorf("setting up gateway webhook: %w", err)
	}
	if err := ctrl.SetupPeeringWebhookWith(mgr); err != nil {
		return fmt.Errorf("setting up peering webhook: %w", err)
	}
	if err := ctrl.SetupVPCInfoWebhookWith(mgr); err != nil {
		return fmt.Errorf("setting up vpcinfo webhook: %w", err)
	}

	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("setting up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("setting up ready check: %w", err)
	}

	slog.Info("Starting manager")

	if err := mgr.Start(kctrl.SetupSignalHandler()); err != nil {
		return fmt.Errorf("running manager: %w", err)
	}

	return nil
}
