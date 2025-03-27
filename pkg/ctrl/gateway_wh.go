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

// +kubebuilder:webhook:path=/mutate-gateway-githedgehog-com-v1beta1-gateway,mutating=true,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gateways,verbs=create;update;delete,versions=v1beta1,name=mgateway.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-gateway-githedgehog-com-v1beta1-gateway,mutating=false,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=gateways,verbs=create;update;delete,versions=v1beta1,name=vgateway.kb.io,admissionReviewVersions=v1

type GatewayWebhook struct {
	client.Reader
}

func SetupGatewayWebhookWith(mgr ctrl.Manager) error {
	w := &GatewayWebhook{
		Reader: mgr.GetClient(),
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&gwapi.Gateway{}).
		WithDefaulter(FromTypedDefaulter(w)).
		WithValidator(FromTypedValidator(w)).
		Complete(); err != nil {
		return fmt.Errorf("creating webhook: %w", err) //nolint:goerr113
	}

	return nil
}

func (w *GatewayWebhook) Default(_ context.Context, obj *gwapi.Gateway) error {
	obj.Default()

	return nil
}

func (w *GatewayWebhook) ValidateCreate(ctx context.Context, obj *gwapi.Gateway) (admission.Warnings, error) {
	return nil, obj.Validate(ctx, w.Reader) //nolint:wrapcheck
}

func (w *GatewayWebhook) ValidateUpdate(ctx context.Context, oldObj *gwapi.Gateway, newObj *gwapi.Gateway) (admission.Warnings, error) {
	// TODO validate diff between oldObj and newObj if needed

	return nil, newObj.Validate(ctx, w.Reader) //nolint:wrapcheck
}

func (w *GatewayWebhook) ValidateDelete(_ context.Context, _ *gwapi.Gateway) (admission.Warnings, error) {
	return nil, nil
}
