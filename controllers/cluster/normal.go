package cluster

import (
	"context"
	"time"

	"github.com/giantswarm/microerror"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/giantswarm/dns-operator-openstack/pkg/cloud"
	"github.com/giantswarm/dns-operator-openstack/pkg/cloud/services/route53"
	"github.com/giantswarm/dns-operator-openstack/pkg/key"
)

func (r *Reconciler) reconcileNormal(ctx context.Context, clusterScope cloud.ClusterScoper) (reconcile.Result, error) {
	clusterScope.Info("reconciling normal")

	// If the openstackCluster doesn't have the finalizer, add it.
	openstackCluster := clusterScope.InfrastructureCluster()
	if !controllerutil.ContainsFinalizer(openstackCluster, key.DNSFinalizerName) {
		clusterScope.Info("adding finalizer")
		controllerutil.AddFinalizer(openstackCluster, key.DNSFinalizerName)
		// Register the finalizer immediately to avoid orphaning openstack resources on delete
		if err := r.client.Update(ctx, openstackCluster); err != nil {
			return reconcile.Result{}, microerror.Mask(err)
		}
		clusterScope.Info("added finalizer")
	}

	route53Service := route53.NewService(clusterScope)
	err := route53Service.ReconcileRoute53(ctx)
	if route53.IsIngressNotReady(err) {
		clusterScope.Info("ingress is not ready yet")
		return reconcile.Result{Requeue: true}, nil
	} else if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	clusterScope.Info("reconciled normal")

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Minute * 5,
	}, nil
}
