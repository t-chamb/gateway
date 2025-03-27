package ctrl

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	gwapi "go.githedgehog.com/gateway/api/gateway/v1alpha1"
)

type GatewayReconciler struct {
	client.Client
}

func SetupGatewayReconcilerWith(mgr ctrl.Manager) error {
	r := &GatewayReconciler{
		Client: mgr.GetClient(),
	}

	if err := builder.TypedControllerManagedBy[*gwapi.Gateway](mgr).
		Named("Gateway").
		For(&gwapi.Gateway{}).
		Complete(r); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return nil
}

func (r *GatewayReconciler) Reconcile(ctx context.Context, req *gwapi.Gateway) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	l.Info("Reconciling Gateways not implemented yet", "namespace", req.Namespace, "name", req.Name)

	return ctrl.Result{}, nil
}
