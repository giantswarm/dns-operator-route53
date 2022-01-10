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

	"github.com/giantswarm/dns-operator-openstack/pkg/cloud/scope"
	"github.com/giantswarm/dns-operator-openstack/pkg/cloud/services/route53"
	"github.com/giantswarm/dns-operator-openstack/pkg/key"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	capo "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// OpenstackClusterReconciler reconciles a openstackCluster object
type OpenstackClusterReconciler struct {
	client.Client

	Log        logr.Logger
	BaseDomain string
	Scheme     *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=openstackclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=openstackclusters/status,verbs=get;update;patch

func (r *OpenstackClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("openstackcluster", req.NamespacedName)

	var infraCluster capo.OpenStackCluster
	err := r.Get(ctx, req.NamespacedName, &infraCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, microerror.Mask(err)
	}

	// Fetch the owner cluster.
	coreCluster, err := util.GetOwnerCluster(ctx, r.Client, infraCluster.ObjectMeta)
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}
	if coreCluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return reconcile.Result{}, microerror.Mask(err)
	}

	log = log.WithValues("cluster", coreCluster.Name)

	// Return early if the core or infrastructure cluster is paused.
	if annotations.IsPaused(coreCluster, &infraCluster) {
		log.Info("infrastructure or core cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	// Create the cluster scope.
	clusterScope, err := scope.NewClusterScope(ctx, scope.ClusterScopeParams{
		BaseDomain:            r.BaseDomain,
		Logger:                log,
		CoreCluster:           coreCluster,
		InfrastructureCluster: &infraCluster,
	})
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	var result ctrl.Result
	if !infraCluster.DeletionTimestamp.IsZero() {
		// Handle deleted clusters
		result, err = r.reconcileDelete(ctx, clusterScope)
	} else {
		// Handle non-deleted clusters
		result, err = r.reconcileNormal(ctx, clusterScope)
	}

	if err != nil {
		log.Error(err, microerror.Pretty(err, true))
	}

	return result, err
}

func (r *OpenstackClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capo.OpenStackCluster{}).
		Complete(r)
}

func (r *OpenstackClusterReconciler) reconcileNormal(ctx context.Context, clusterScope *scope.ClusterScope) (reconcile.Result, error) {
	clusterScope.Info("Reconciling openstackCluster normal")

	openstackCluster := clusterScope.InfrastructureCluster()
	// If the openstackCluster doesn't have the finalizer, add it.
	controllerutil.AddFinalizer(openstackCluster, key.DNSFinalizerName)
	// Register the finalizer immediately to avoid orphaning openstack resources on delete
	if err := r.Update(ctx, openstackCluster); err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	route53Service := route53.NewService(clusterScope)
	err := route53Service.ReconcileRoute53(ctx)
	if route53.IsIngressNotReady(err) {
		clusterScope.Error(err, "ingress is not ready yet, requeing")
		return reconcile.Result{Requeue: true}, microerror.Mask(err)
	} else if err != nil {
		clusterScope.Error(err, "error creating route53")
		return reconcile.Result{}, microerror.Mask(err)
	}

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Minute * 5,
	}, nil
}

func (r *OpenstackClusterReconciler) reconcileDelete(ctx context.Context, clusterScope *scope.ClusterScope) (reconcile.Result, error) {
	clusterScope.Info("Reconciling openstackCluster delete")

	route53Service := route53.NewService(clusterScope)

	if err := route53Service.DeleteRoute53(ctx); err != nil {
		clusterScope.Error(err, "error deleting route53")
		return reconcile.Result{}, microerror.Mask(err)
	}

	openstackCluster := clusterScope.InfrastructureCluster()
	// openstackCluster is deleted so remove the finalizer.
	controllerutil.RemoveFinalizer(openstackCluster, key.DNSFinalizerName)
	// Finally remove the finalizer
	if err := r.Update(ctx, openstackCluster); err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Minute * 5,
	}, nil
}
