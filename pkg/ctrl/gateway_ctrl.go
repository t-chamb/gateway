// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	gwapi "go.githedgehog.com/gateway/api/gateway/v1alpha1"
)

// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways/finalizers,verbs=update

// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=peerings,verbs=get;list;watch

type GatewayReconciler struct {
	client.Client
}

func SetupGatewayReconcilerWith(mgr ctrl.Manager) error {
	r := &GatewayReconciler{
		Client: mgr.GetClient(),
	}

	if err := ctrl.NewControllerManagedBy(mgr).
		Named("Gateway").
		For(&gwapi.Gateway{}).
		Complete(r); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return nil
}

func (r *GatewayReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	l.Info("Reconciling Gateways not implemented yet", "namespace", req.Namespace, "name", req.Name)

	return ctrl.Result{}, nil
}
