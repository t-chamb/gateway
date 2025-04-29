// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"strings"

	gwapi "go.githedgehog.com/gateway/api/gateway/v1alpha1"
	gwintapi "go.githedgehog.com/gateway/api/gwint/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents/finalizers,verbs=update

// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfoes,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=peerings,verbs=get;list;watch

// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete

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
		Watches(&gwapi.Peering{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllGateways)).
		Watches(&gwapi.VPCInfo{}, handler.EnqueueRequestsFromMapFunc(r.enqueueAllGateways)).
		Complete(r); err != nil {
		return fmt.Errorf("setting up controller: %w", err)
	}

	return nil
}

func (r *GatewayReconciler) enqueueAllGateways(ctx context.Context, obj kclient.Object) []reconcile.Request {
	res := []reconcile.Request{}

	gws := &gwapi.GatewayList{}
	if err := r.List(ctx, gws, kclient.InNamespace(obj.GetNamespace())); err != nil {
		kctrllog.FromContext(ctx).Error(err, "error listing gateways to reconcile all")

		return nil
	}

	for _, sw := range gws.Items {
		res = append(res, reconcile.Request{NamespacedName: ktypes.NamespacedName{
			Namespace: sw.Namespace,
			Name:      sw.Name,
		}})
	}

	return res
}

func (r *GatewayReconciler) Reconcile(ctx context.Context, req kctrl.Request) (kctrl.Result, error) {
	l := kctrllog.FromContext(ctx)

	gw := &gwapi.Gateway{}
	if err := r.Get(ctx, req.NamespacedName, gw); err != nil {
		return kctrl.Result{}, fmt.Errorf("getting gateway: %w", err)
	}

	if gw.DeletionTimestamp != nil {
		l.Info("Gateway is being deleted, skipping", "name", req.Name, "namespace", req.Namespace)

		return kctrl.Result{}, nil
	}

	l.Info("Reconciling Gateway", "name", req.Name, "namespace", req.Namespace)

	if err := r.prepareAgentInfra(ctx, gw); err != nil {
		return kctrl.Result{}, fmt.Errorf("preparing agent infra: %w", err)
	}

	vpcList := &gwapi.VPCInfoList{}
	if err := r.List(ctx, vpcList); err != nil {
		return kctrl.Result{}, fmt.Errorf("listing vpcinfos: %w", err)
	}
	vpcs := map[string]gwintapi.VPCInfoData{}
	for _, vpc := range vpcList.Items {
		if !vpc.IsReady() {
			l.Info("VPCInfo not ready, skipping", "name", vpc.Name, "namespace", vpc.Namespace)

			continue
		}
		vpcs[vpc.Name] = gwintapi.VPCInfoData{
			VPCInfoSpec:   vpc.Spec,
			VPCInfoStatus: vpc.Status,
		}
	}

	peeringList := &gwapi.PeeringList{}
	if err := r.List(ctx, peeringList); err != nil {
		return kctrl.Result{}, fmt.Errorf("listing peerings: %w", err)
	}
	peerings := map[string]gwapi.PeeringSpec{}
	for _, peering := range peeringList.Items {
		peerings[peering.Name] = peering.Spec
	}

	gwAg := &gwintapi.GatewayAgent{ObjectMeta: kmetav1.ObjectMeta{Namespace: gw.Namespace, Name: gw.Name}}
	if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, gwAg, func() error {
		// TODO consider blocking owner deletion, would require foregroundDeletion finalizer on the owner
		if err := ctrlutil.SetControllerReference(gw, gwAg, r.Scheme(),
			ctrlutil.WithBlockOwnerDeletion(false)); err != nil {
			return fmt.Errorf("setting controller reference: %w", err)
		}

		gwAg.Spec.VPCs = vpcs
		gwAg.Spec.Peerings = peerings

		return nil
	}); err != nil {
		return kctrl.Result{}, fmt.Errorf("creating or updating gateway agent: %w", err)
	}

	return kctrl.Result{}, nil
}

func entityName(gwName string, t ...string) string {
	if len(t) == 0 {
		return fmt.Sprintf("gw-%s", gwName)
	}

	return fmt.Sprintf("gw--%s--%s", gwName, strings.Join(t, "-"))
}

func (r *GatewayReconciler) prepareAgentInfra(ctx context.Context, gw *gwapi.Gateway) error {
	name := entityName(gw.Name)

	sa := &corev1.ServiceAccount{ObjectMeta: kmetav1.ObjectMeta{Namespace: gw.Namespace, Name: name}}
	_, err := ctrlutil.CreateOrUpdate(ctx, r.Client, sa, func() error { return nil })
	if err != nil {
		return fmt.Errorf("creating service account: %w", err)
	}

	role := &rbacv1.Role{ObjectMeta: kmetav1.ObjectMeta{Namespace: gw.Namespace, Name: name}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, role, func() error {
		role.Rules = []rbacv1.PolicyRule{
			{
				APIGroups:     []string{gwintapi.GroupVersion.Group},
				Resources:     []string{"gatewayagents"},
				ResourceNames: []string{gw.Name},
				Verbs:         []string{"get", "watch"},
			},
			{
				APIGroups:     []string{gwintapi.GroupVersion.Group},
				Resources:     []string{"gatewayagents/status"},
				ResourceNames: []string{gw.Name},
				Verbs:         []string{"get", "update", "patch"},
			},
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("creating role: %w", err)
	}

	roleBinding := &rbacv1.RoleBinding{ObjectMeta: kmetav1.ObjectMeta{Namespace: gw.Namespace, Name: name}}
	_, err = ctrlutil.CreateOrUpdate(ctx, r.Client, roleBinding, func() error {
		roleBinding.Subjects = []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      sa.Name,
				Namespace: sa.Namespace,
			},
		}
		roleBinding.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     role.Name,
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("creating role binding: %w", err)
	}

	return nil
}
