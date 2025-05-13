// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/samber/lo"
	gwapi "go.githedgehog.com/gateway/api/gateway/v1alpha1"
	gwintapi "go.githedgehog.com/gateway/api/gwint/v1alpha1"
	"go.githedgehog.com/gateway/api/meta"
	"go.githedgehog.com/gateway/pkg/agent"
	appv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	kapierrors "k8s.io/apimachinery/pkg/api/errors"
	kmetav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	kctrl "sigs.k8s.io/controller-runtime"
	kclient "sigs.k8s.io/controller-runtime/pkg/client"
	ctrlutil "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	kctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	kyaml "sigs.k8s.io/yaml"
)

const (
	configVolumeName = "config"

	dataplaneRunVolumeName = "dataplane-run"
	frrRunVolumeName       = "frr-run"

	dataplaneRunHostPath = "/run/hedgehog/dataplane"
	frrRunHostPath       = "/run/hedgehog/frr"

	dataplaneRunMountPath = "/run/dataplane"
	dataplaneSocketName   = "dataplane.sock"

	frrTmpVolumeName = "frr-tmp"
)

// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gwint.githedgehog.com,resources=gatewayagents/finalizers,verbs=update

// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=gateways/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=vpcinfos,verbs=get;list;watch
// +kubebuilder:rbac:groups=gateway.githedgehog.com,resources=peerings,verbs=get;list;watch

// +kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=daemonsets,verbs=get;list;watch;create;update;patch;delete

type GatewayReconciler struct {
	kclient.Client
	cfg *meta.GatewayCtrlConfig
}

func SetupGatewayReconcilerWith(mgr kctrl.Manager, cfg *meta.GatewayCtrlConfig) error {
	if cfg == nil {
		return fmt.Errorf("gateway controller config is nil") //nolint:goerr113
	}

	r := &GatewayReconciler{
		Client: mgr.GetClient(),
		cfg:    cfg,
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

	if req.Namespace != r.cfg.Namespace {
		l.Info("Skipping Gateway in unexpected namespace", "name", req.Name, "namespace", req.Namespace)

		return kctrl.Result{}, nil
	}

	gw := &gwapi.Gateway{}
	if err := r.Get(ctx, req.NamespacedName, gw); err != nil {
		if kapierrors.IsNotFound(err) {
			return kctrl.Result{}, nil
		}

		return kctrl.Result{}, fmt.Errorf("getting gateway: %w", err)
	}

	if gw.DeletionTimestamp != nil {
		l.Info("Gateway is being deleted, skipping", "name", req.Name, "namespace", req.Namespace)

		return kctrl.Result{}, nil
	}

	l.Info("Reconciling Gateway", "name", req.Name, "namespace", req.Namespace)

	vpcList := &gwapi.VPCInfoList{}
	if err := r.List(ctx, vpcList); err != nil {
		return kctrl.Result{}, fmt.Errorf("listing vpcinfos: %w", err)
	}
	vpcs := map[string]gwintapi.VPCInfoData{}
	for _, vpc := range vpcList.Items {
		if !vpc.IsReady() {
			l.Info("VPCInfo not ready, retrying", "name", vpc.Name, "namespace", vpc.Namespace)

			// TODO consider ignoring non-ready VPCs
			return kctrl.Result{Requeue: true, RequeueAfter: 1 * time.Second}, nil
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
		missingVPC := false

		for peerVPC := range peering.Spec.Peering {
			if _, exists := vpcs[peerVPC]; !exists {
				l.Info("Peered VPC not found, skipping", "peering", peering.Name, "vpc", peerVPC, "ns", peering.Namespace)

				missingVPC = true

				break
			}
		}

		if missingVPC {
			continue
		}

		peerings[peering.Name] = peering.Spec
	}

	gwAg := &gwintapi.GatewayAgent{ObjectMeta: kmetav1.ObjectMeta{Namespace: gw.Namespace, Name: gw.Name}}
	if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, gwAg, func() error {
		// TODO consider blocking owner deletion, would require foregroundDeletion finalizer on the owner
		if err := ctrlutil.SetControllerReference(gw, gwAg, r.Scheme(),
			ctrlutil.WithBlockOwnerDeletion(false)); err != nil {
			return fmt.Errorf("setting controller reference: %w", err)
		}

		gwAg.Spec.Gateway = gw.Spec
		gwAg.Spec.VPCs = vpcs
		gwAg.Spec.Peerings = peerings

		return nil
	}); err != nil {
		return kctrl.Result{}, fmt.Errorf("creating or updating gateway agent: %w", err)
	}

	if err := r.deployGateway(ctx, gw); err != nil {
		return kctrl.Result{}, fmt.Errorf("deploying gateway: %w", err)
	}

	return kctrl.Result{}, nil
}

func entityName(gwName string, t ...string) string {
	if len(t) == 0 {
		return fmt.Sprintf("gw-%s", gwName)
	}

	return fmt.Sprintf("gw--%s--%s", gwName, strings.Join(t, "-"))
}

func (r *GatewayReconciler) deployGateway(ctx context.Context, gw *gwapi.Gateway) error {
	saName := entityName(gw.Name)

	{
		sa := &corev1.ServiceAccount{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: gw.Namespace,
			Name:      saName,
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, sa, func() error { return nil }); err != nil {
			return fmt.Errorf("creating service account: %w", err)
		}

		role := &rbacv1.Role{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: gw.Namespace,
			Name:      saName,
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, role, func() error {
			if err := ctrlutil.SetControllerReference(gw, role, r.Scheme(),
				ctrlutil.WithBlockOwnerDeletion(false)); err != nil {
				return fmt.Errorf("setting controller reference: %w", err)
			}

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
		}); err != nil {
			return fmt.Errorf("creating role: %w", err)
		}

		roleBinding := &rbacv1.RoleBinding{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: gw.Namespace,
			Name:      saName,
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, roleBinding, func() error {
			if err := ctrlutil.SetControllerReference(gw, roleBinding, r.Scheme(),
				ctrlutil.WithBlockOwnerDeletion(false)); err != nil {
				return fmt.Errorf("setting controller reference: %w", err)
			}

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
		}); err != nil {
			return fmt.Errorf("creating role binding: %w", err)
		}
	}

	replaceUpdateStrategy := appv1.DaemonSetUpdateStrategy{
		Type: appv1.RollingUpdateDaemonSetStrategyType,
		RollingUpdate: &appv1.RollingUpdateDaemonSet{
			MaxUnavailable: ptr.To(intstr.FromInt(1)),
			MaxSurge:       ptr.To(intstr.FromInt(0)),
		},
	}

	dataplaneSocketVolume := corev1.Volume{
		Name: dataplaneRunVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: dataplaneRunHostPath,
				Type: ptr.To(corev1.HostPathDirectoryOrCreate),
			},
		},
	}

	{
		agCfgData, err := kyaml.Marshal(&meta.AgentConfig{
			Name:      gw.Name,
			Namespace: gw.Namespace,
			// TODO switch to unix socket: "unix://" + filepath.Join(dataplaneRunMountDir, dataplaneSocketName),
			DataplaneAddress: "[::1]:50051",
		})
		if err != nil {
			return fmt.Errorf("marshalling agent config: %w", err)
		}

		agCM := &corev1.ConfigMap{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: gw.Namespace,
			Name:      entityName(gw.Name, "agent"),
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, agCM, func() error {
			if err := ctrlutil.SetControllerReference(gw, agCM, r.Scheme(),
				ctrlutil.WithBlockOwnerDeletion(false)); err != nil {
				return fmt.Errorf("setting controller reference: %w", err)
			}

			agCM.Data = map[string]string{
				agent.ConfigFile: string(agCfgData),
			}

			return nil
		}); err != nil {
			return fmt.Errorf("creating or updating gateway agent configmap: %w", err)
		}
	}

	{
		agDS := &appv1.DaemonSet{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: gw.Namespace,
			Name:      entityName(gw.Name, "agent"),
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, agDS, func() error {
			if err := ctrlutil.SetControllerReference(gw, agDS, r.Scheme(),
				ctrlutil.WithBlockOwnerDeletion(false)); err != nil {
				return fmt.Errorf("setting controller reference: %w", err)
			}

			labels := map[string]string{
				"app.kubernetes.io/name": agDS.Name, // TODO
			}

			agDS.Spec = appv1.DaemonSetSpec{
				Selector: &kmetav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: kmetav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.PodSpec{
						NodeName:                      gw.Name,
						ServiceAccountName:            saName,
						HostNetwork:                   true,
						DNSPolicy:                     corev1.DNSClusterFirstWithHostNet,
						TerminationGracePeriodSeconds: ptr.To(int64(10)),
						Tolerations:                   r.cfg.Tolerations,
						Containers: []corev1.Container{
							{
								Name:  "agent",
								Image: r.cfg.AgentRef,
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      dataplaneRunVolumeName,
										MountPath: dataplaneRunMountPath,
									},
									{
										Name:      configVolumeName,
										MountPath: agent.ConfigDir,
										ReadOnly:  true,
									},
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: ptr.To(true),
									RunAsUser:  ptr.To(int64(0)),
								},
							},
						},
						Volumes: []corev1.Volume{
							dataplaneSocketVolume,
							{
								Name: configVolumeName,
								VolumeSource: corev1.VolumeSource{
									ConfigMap: &corev1.ConfigMapVolumeSource{
										LocalObjectReference: corev1.LocalObjectReference{
											Name: entityName(gw.Name, "agent"),
										},
									},
								},
							},
						},
					},
				},
				UpdateStrategy: replaceUpdateStrategy,
			}

			return nil
		}); err != nil {
			return fmt.Errorf("creating or updating gateway agent daemonset: %w", err)
		}
	}

	frrSocketVolume := corev1.Volume{
		Name: frrRunVolumeName,
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: frrRunHostPath,
				Type: ptr.To(corev1.HostPathDirectoryOrCreate),
			},
		},
	}

	{
		ifaceFlags := lo.Flatten(lo.Map(lo.Keys(gw.Spec.Interfaces),
			func(ifaceName string, _ int) []string {
				return []string{"--interface", ifaceName}
			}))

		dpDS := &appv1.DaemonSet{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: gw.Namespace,
			Name:      entityName(gw.Name, "dataplane"),
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, dpDS, func() error {
			if err := ctrlutil.SetControllerReference(gw, dpDS, r.Scheme(),
				ctrlutil.WithBlockOwnerDeletion(false)); err != nil {
				return fmt.Errorf("setting controller reference: %w", err)
			}

			labels := map[string]string{
				"app.kubernetes.io/name": dpDS.Name, // TODO
			}

			dpDS.Spec = appv1.DaemonSetSpec{
				Selector: &kmetav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: kmetav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.PodSpec{
						NodeName:                      gw.Name,
						HostNetwork:                   true,
						DNSPolicy:                     corev1.DNSClusterFirstWithHostNet,
						TerminationGracePeriodSeconds: ptr.To(int64(10)),
						Tolerations:                   r.cfg.Tolerations,
						Containers: []corev1.Container{
							{
								Name:  "dataplane",
								Image: r.cfg.DataplaneRef,
								Args:  append([]string{"--driver", "kernel"}, ifaceFlags...),
								SecurityContext: &corev1.SecurityContext{
									Privileged: ptr.To(true),
									RunAsUser:  ptr.To(int64(0)),
								},
								VolumeMounts: []corev1.VolumeMount{
									{
										Name:      dataplaneRunVolumeName,
										MountPath: "/run/dataplane",
									},
									{
										Name:      frrRunVolumeName,
										MountPath: "/var/run/frr",
									},
								},
							},
						},
						Volumes: []corev1.Volume{
							dataplaneSocketVolume,
							frrSocketVolume,
						},
					},
				},
				UpdateStrategy: replaceUpdateStrategy,
			}

			return nil
		}); err != nil {
			return fmt.Errorf("creating or updating gateway dataplane daemonset: %w", err)
		}
	}

	frrVolumeMounts := []corev1.VolumeMount{
		{
			Name:      frrRunVolumeName,
			MountPath: "/run/frr",
		},
		{
			Name:      frrTmpVolumeName,
			MountPath: "/var/tmp/frr",
		},
	}

	{
		frrDS := &appv1.DaemonSet{ObjectMeta: kmetav1.ObjectMeta{
			Namespace: gw.Namespace,
			Name:      entityName(gw.Name, "frr"),
		}}
		if _, err := ctrlutil.CreateOrUpdate(ctx, r.Client, frrDS, func() error {
			if err := ctrlutil.SetControllerReference(gw, frrDS, r.Scheme(),
				ctrlutil.WithBlockOwnerDeletion(false)); err != nil {
				return fmt.Errorf("setting controller reference: %w", err)
			}

			labels := map[string]string{
				"app.kubernetes.io/name": frrDS.Name, // TODO
			}

			frrDS.Spec = appv1.DaemonSetSpec{
				Selector: &kmetav1.LabelSelector{
					MatchLabels: labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: kmetav1.ObjectMeta{
						Labels: labels,
					},
					Spec: corev1.PodSpec{
						NodeName:                      gw.Name,
						HostNetwork:                   true,
						DNSPolicy:                     corev1.DNSClusterFirstWithHostNet,
						TerminationGracePeriodSeconds: ptr.To(int64(10)),
						Tolerations:                   r.cfg.Tolerations,
						InitContainers: []corev1.Container{
							// TODO remove it after frr container will take care of this
							{
								Name:    "init-frr",
								Image:   r.cfg.FRRRef,
								Command: []string{"/bin/bash", "-c", "--"},
								Args: []string{
									"set -ex && " +
										"rm -rf /var/run && ln -s /run /var/run &&" +
										"mkdir -p /var/run/frr/hh && chown -R frr:frr /var/run/frr/ && chmod -R 760 /var/run/frr",
								},
								SecurityContext: &corev1.SecurityContext{
									Privileged: ptr.To(true),
									RunAsUser:  ptr.To(int64(0)),
								},
								VolumeMounts: frrVolumeMounts,
							},
						},
						Containers: []corev1.Container{
							{
								Name:    "frr",
								Image:   r.cfg.FRRRef,
								Command: []string{"/libexec/frr/docker-start"},
								Args:    []string{"--sock-path", "/var/run/frr/frr-agent.sock", "--reloader", "/libexec/frr/frr-reload", "--bindir", "/bin"},
								SecurityContext: &corev1.SecurityContext{
									Privileged: ptr.To(true),
									RunAsUser:  ptr.To(int64(0)),
								},
								VolumeMounts: frrVolumeMounts,
							},
						},
						Volumes: []corev1.Volume{
							frrSocketVolume,
							{
								Name: frrTmpVolumeName,
								VolumeSource: corev1.VolumeSource{
									// TODO consider memory medium
									EmptyDir: &corev1.EmptyDirVolumeSource{},
								},
							},
						},
					},
				},
				UpdateStrategy: replaceUpdateStrategy,
			}

			return nil
		}); err != nil {
			return fmt.Errorf("creating or updating gateway frr daemonset: %w", err)
		}
	}

	return nil
}
