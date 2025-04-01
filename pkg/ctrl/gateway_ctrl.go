// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	gwapi "go.githedgehog.com/gateway/api/gateway/v1alpha1"
)

// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways/finalizers,verbs=update

// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfoes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=peerings,verbs=get;list;watch

type GatewayReconciler struct {
	kclient.Client
}

func SetupGatewayReconcilerWith(mgr kctrl.Manager) error {
	r := &GatewayReconciler{
		Client: mgr.GetClient(),
	}

	if err := kctrl.NewControllerManagedBy(mgr).
		Named("Gateway").
		For(&gwapi.Gateway{}).
		Complete(r); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return nil
}

func (r *GatewayReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	l.Info("Reconciling Gateways not implemented yet", "namespace", req.Namespace, "name", req.Name)

	return kctrl.Result{}, nil
}
