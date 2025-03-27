// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	gwapi "go.githedgehog.com/gateway/api/gateway/v1alpha1"
)

// +kubebuilder:webhook:path=/mutate-gateway-githedgehog-com-v1alpha1-peering,mutating=true,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=peerings,verbs=create;update;delete,versions=v1alpha1,name=mpeering.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-gateway-githedgehog-com-v1alpha1-peering,mutating=false,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=peerings,verbs=create;update;delete,versions=v1alpha1,name=vpeering.kb.io,admissionReviewVersions=v1

type PeeringWebhook struct {
	client.Reader
}

func SetupPeeringWebhookWith(mgr ctrl.Manager) error {
	w := &PeeringWebhook{
		Reader: mgr.GetClient(),
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&gwapi.Peering{}).
		WithDefaulter(FromTypedDefaulter(w)).
		WithValidator(FromTypedValidator(w)).
		Complete(); err != nil {
		return fmt.Errorf("creating webhook: %w", err) //nolint:goerr113
	}

	return nil
}

func (w *PeeringWebhook) Default(_ context.Context, obj *gwapi.Peering) error {
	obj.Default()

	return nil
}

func (w *PeeringWebhook) ValidateCreate(ctx context.Context, obj *gwapi.Peering) (admission.Warnings, error) {
	return nil, obj.Validate(ctx, w.Reader) //nolint:wrapcheck
}

func (w *PeeringWebhook) ValidateUpdate(ctx context.Context, oldObj *gwapi.Peering, newObj *gwapi.Peering) (admission.Warnings, error) {
	// TODO validate diff between oldObj and newObj if needed

	return nil, newObj.Validate(ctx, w.Reader) //nolint:wrapcheck
}

func (w *PeeringWebhook) ValidateDelete(_ context.Context, _ *gwapi.Peering) (admission.Warnings, error) {
	return nil, nil
}
