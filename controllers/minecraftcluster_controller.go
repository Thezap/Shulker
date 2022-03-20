/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"

	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	shulkermciov1alpha1 "shulkermc.io/m/v2/api/v1alpha1"
	resource "shulkermc.io/m/v2/internal/resource/cluster"
)

// MinecraftClusterReconciler reconciles a MinecraftCluster object
type MinecraftClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update
//+kubebuilder:rbac:groups=shulkermc.io,resources=minecraftclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=shulkermc.io,resources=minecraftclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=shulkermc.io,resources=minecraftclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=shulkermc.io,resources=minecraftservers,verbs=get;list;watch

func (r *MinecraftClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	minecraftCluster, err := r.getMinecraftCluster(ctx, req.NamespacedName)

	if client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	} else if k8serrors.IsNotFound(err) {
		// No need to requeue if the resource no longer exists
		return ctrl.Result{}, nil
	}

	// Check if the resource has been marked for deletion
	if !minecraftCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		logger.Info("Deleting")
		return ctrl.Result{}, r.prepareForDeletion(ctx, minecraftCluster)
	}

	if err := r.addFinalizerIfNeeded(ctx, minecraftCluster); err != nil {
		return ctrl.Result{}, err
	}

	resourceBuilder := resource.MinecraftClusterResourceBuilder{
		Instance: minecraftCluster,
		Scheme:   r.Scheme,
	}
	builders, dirtyBuilders := resourceBuilder.ResourceBuilders()

	err = ReconcileWithResourceBuilders(r.Client, ctx, builders, dirtyBuilders)
	if err != nil {
		return ctrl.Result{}, err
	}

	proxyDeploymentList, err := r.listProxyDeployments(ctx, minecraftCluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	minecraftCluster.Status.Proxies = 0
	for _, proxyDeployment := range proxyDeploymentList.Items {
		if meta.IsStatusConditionTrue(proxyDeployment.Status.Conditions, string(shulkermciov1alpha1.ProxyDeploymentReadyCondition)) {
			minecraftCluster.Status.Proxies += proxyDeployment.Status.Replicas
		}
	}

	minecraftServerDeploymentList, err := r.listMinecraftServerDeployments(ctx, minecraftCluster)
	if err != nil {
		return ctrl.Result{}, err
	}
	minecraftCluster.Status.Servers = 0
	for _, minecraftServerDeployment := range minecraftServerDeploymentList.Items {
		if meta.IsStatusConditionTrue(minecraftServerDeployment.Status.Conditions, string(shulkermciov1alpha1.MinecraftServerDeploymentReadyCondition)) {
			minecraftCluster.Status.Servers += minecraftServerDeployment.Status.Replicas
		}
	}

	minecraftCluster.Status.SetCondition(shulkermciov1alpha1.ClusterReadyCondition, metav1.ConditionTrue, "Ready", "Cluster is ready")

	err = r.Status().Update(ctx, minecraftCluster)
	return ctrl.Result{}, err
}

func (r *MinecraftClusterReconciler) listProxyDeployments(ctx context.Context, minecraftCluster *shulkermciov1alpha1.MinecraftCluster) (*shulkermciov1alpha1.ProxyDeploymentList, error) {
	list := shulkermciov1alpha1.ProxyDeploymentList{}
	err := r.List(ctx, &list, client.InNamespace(minecraftCluster.Namespace), client.MatchingFields{
		".spec.minecraftClusterRef.name": minecraftCluster.Name,
	})

	if err != nil {
		return nil, err
	}

	return &list, nil
}

func (r *MinecraftClusterReconciler) listMinecraftServerDeployments(ctx context.Context, minecraftCluster *shulkermciov1alpha1.MinecraftCluster) (*shulkermciov1alpha1.MinecraftServerDeploymentList, error) {
	list := shulkermciov1alpha1.MinecraftServerDeploymentList{}
	err := r.List(ctx, &list, client.InNamespace(minecraftCluster.Namespace), client.MatchingFields{
		".spec.minecraftClusterRef.name": minecraftCluster.Name,
	})

	if err != nil {
		return nil, err
	}

	return &list, nil
}

func (r *MinecraftClusterReconciler) getMinecraftCluster(ctx context.Context, namespacedName types.NamespacedName) (*shulkermciov1alpha1.MinecraftCluster, error) {
	minecraftClusterInstance := &shulkermciov1alpha1.MinecraftCluster{}
	err := r.Get(ctx, namespacedName, minecraftClusterInstance)
	return minecraftClusterInstance, err
}

func (r *MinecraftClusterReconciler) findMinecraftClusterForProxyDeployment(object client.Object) []reconcile.Request {
	proxyDeployment := object.(*shulkermciov1alpha1.ProxyDeployment)

	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: proxyDeployment.GetNamespace(),
			Name:      proxyDeployment.Spec.ClusterRef.Name,
		},
	}}
}

func (r *MinecraftClusterReconciler) findMinecraftClusterForMinecraftServerDeployment(object client.Object) []reconcile.Request {
	minecraftServerDeployment := object.(*shulkermciov1alpha1.MinecraftServerDeployment)

	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Namespace: minecraftServerDeployment.GetNamespace(),
			Name:      minecraftServerDeployment.Spec.ClusterRef.Name,
		},
	}}
}

func (r *MinecraftClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&shulkermciov1alpha1.MinecraftCluster{}).
		Owns(&shulkermciov1alpha1.MinecraftServerDeployment{}).
		Owns(&rbacv1.Role{}).
		Watches(
			&source.Kind{Type: &shulkermciov1alpha1.ProxyDeployment{}},
			handler.EnqueueRequestsFromMapFunc(r.findMinecraftClusterForProxyDeployment),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Watches(
			&source.Kind{Type: &shulkermciov1alpha1.MinecraftServerDeployment{}},
			handler.EnqueueRequestsFromMapFunc(r.findMinecraftClusterForMinecraftServerDeployment),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		Complete(r)
}
