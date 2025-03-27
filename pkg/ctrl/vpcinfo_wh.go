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

// +kubebuilder:webhook:path=/mutate-gateway-githedgehog-com-v1beta1-vpcinfo,mutating=true,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=vpcinfos,verbs=create;update;delete,versions=v1beta1,name=mvpcinfo.kb.io,admissionReviewVersions=v1
// +kubebuilder:webhook:path=/validate-gateway-githedgehog-com-v1beta1-vpcinfo,mutating=false,failurePolicy=fail,sideEffects=None,groups=gateway.githedgehog.com,resources=vpcinfos,verbs=create;update;delete,versions=v1beta1,name=vvpcinfo.kb.io,admissionReviewVersions=v1

type VPCInfoWebhook struct {
	client.Reader
}

func SetupVPCInfoWebhookWith(mgr ctrl.Manager) error {
	w := &VPCInfoWebhook{
		Reader: mgr.GetClient(),
	}

	if err := ctrl.NewWebhookManagedBy(mgr).
		For(&gwapi.VPCInfo{}).
		WithDefaulter(FromTypedDefaulter(w)).
		WithValidator(FromTypedValidator(w)).
		Complete(); err != nil {
		return fmt.Errorf("creating webhook: %w", err) //nolint:goerr113
	}

	return nil
}

func (w *VPCInfoWebhook) Default(_ context.Context, obj *gwapi.VPCInfo) error {
	obj.Default()

	return nil
}

func (w *VPCInfoWebhook) ValidateCreate(ctx context.Context, obj *gwapi.VPCInfo) (admission.Warnings, error) {
	return nil, obj.Validate(ctx, w.Reader) //nolint:wrapcheck
}

func (w *VPCInfoWebhook) ValidateUpdate(ctx context.Context, oldObj *gwapi.VPCInfo, newObj *gwapi.VPCInfo) (admission.Warnings, error) {
	// TODO validate diff between oldObj and newObj if needed

	return nil, newObj.Validate(ctx, w.Reader) //nolint:wrapcheck
}

func (w *VPCInfoWebhook) ValidateDelete(_ context.Context, _ *gwapi.VPCInfo) (admission.Warnings, error) {
	return nil, nil
}
