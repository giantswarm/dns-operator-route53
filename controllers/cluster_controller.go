/*


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
	"time"

	"github.com/giantswarm/dns-operator-route53/pkg/cloud/scope"
	"github.com/giantswarm/dns-operator-route53/pkg/cloud/services/route53"
	"github.com/giantswarm/dns-operator-route53/pkg/key"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/giantswarm/microerror"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client

	BaseDomain        string
	ManagementCluster string
}

func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	log.WithValues("cluster", req.NamespacedName)

	cluster, err := util.GetClusterByName(ctx, r.Client, req.Namespace, req.Name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
	}

	// init the unstructured client
	infraCluster := &unstructured.Unstructured{}

	// get the InfrastructureRef (v1.ObjectReference) from the CAPI cluster
	infraRef := cluster.Spec.InfrastructureRef

	// set the GVK to the unstructured infraCluster
	infraCluster.SetGroupVersionKind(infraRef.GroupVersionKind())

	if err := r.Client.Get(ctx, req.NamespacedName, infraCluster); err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	log.WithValues("infrastructure cluster", infraCluster.GetClusterName())
	log.WithValues("infrastructure group", infraCluster.GroupVersionKind().Group, "infrastructure kind", infraCluster.GroupVersionKind().Kind, "infrastructure version", infraCluster.GroupVersionKind().Version)

	// Return early if the core or infrastructure cluster is paused.
	if annotations.IsPaused(cluster, infraCluster) {
		log.Info("infrastructure or core cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	// Create the cluster scope.
	clusterScope, err := scope.NewClusterScope(ctx, scope.ClusterScopeParams{
		BaseDomain:            r.BaseDomain,
		Cluster:               cluster,
		InfrastructureCluster: infraCluster,
		ManagementCluster:     r.ManagementCluster,
	})
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	// Handle deleted clusters
	if !cluster.DeletionTimestamp.IsZero() || !infraCluster.GetDeletionTimestamp().IsZero() {
		return r.reconcileDelete(ctx, clusterScope)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, clusterScope)
}

func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capi.Cluster{}).
		Complete(r)
}

func (r *ClusterReconciler) reconcileNormal(ctx context.Context, clusterScope *scope.ClusterScope) (reconcile.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling cluster normal")

	cluster := clusterScope.Cluster()
	infraCluster := clusterScope.InfrastructureCluster()

	// If the cluster doesn't have the finalizer, add it.
	if !controllerutil.ContainsFinalizer(cluster, key.DNSFinalizerNameNew) {
		controllerutil.AddFinalizer(cluster, key.DNSFinalizerNameNew)
		// Register the finalizer immediately to avoid orphaning cluster resources on delete
		if err := r.Update(ctx, cluster); err != nil {
			return reconcile.Result{}, microerror.Mask(err)
		}
	}

	// Register the finalizer immediately to avoid orphaning infrastructure cluster resources on delete
	if !controllerutil.ContainsFinalizer(infraCluster, key.DNSFinalizerNameNew) {
		controllerutil.AddFinalizer(infraCluster, key.DNSFinalizerNameNew)
		if err := r.Update(ctx, infraCluster); err != nil {
			return reconcile.Result{}, microerror.Mask(err)
		}
	}

	// TODO start: delete after all CAPO based clusters got migrated to new finalizer
	// remove the old finalizer from infrastructure clusters
	if controllerutil.ContainsFinalizer(infraCluster, key.DNSFinalizerNameOld) {
		controllerutil.RemoveFinalizer(infraCluster, key.DNSFinalizerNameOld)
		if err := r.Update(ctx, infraCluster); err != nil {
			return reconcile.Result{}, microerror.Mask(err)
		}
	}
	// TODO end

	route53Service := route53.NewService(clusterScope)
	err := route53Service.ReconcileRoute53(ctx)
	if route53.IsIngressNotReady(err) {
		log.Error(err, "ingress is not ready yet, requeuing")
		return reconcile.Result{}, microerror.Mask(err)
	} else if err != nil {
		log.Error(err, "error creating route53")
		return reconcile.Result{}, microerror.Mask(err)
	}

	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

func (r *ClusterReconciler) reconcileDelete(ctx context.Context, clusterScope *scope.ClusterScope) (reconcile.Result, error) {
	log := log.FromContext(ctx)
	log.Info("Reconciling Cluster delete")

	cluster := clusterScope.Cluster()
	infraCluster := clusterScope.InfrastructureCluster()

	// cluster and infrastructure don't have finalizer. it means deletion is already done.
	if !controllerutil.ContainsFinalizer(cluster, key.DNSFinalizerNameNew) &&
		!controllerutil.ContainsFinalizer(infraCluster, key.DNSFinalizerNameNew) {
		return reconcile.Result{}, nil
	}

	route53Service := route53.NewService(clusterScope)

	if err := route53Service.DeleteRoute53(ctx); err != nil {
		log.Error(err, "error deleting route53")
		return reconcile.Result{}, microerror.Mask(err)
	}

	// cluster is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(cluster, key.DNSFinalizerNameNew)
	if err := r.Update(ctx, cluster); err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	// infrastructrue cluster is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(infraCluster, key.DNSFinalizerNameNew)
	if err := r.Update(ctx, infraCluster); err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Minute * 5,
	}, nil
}
