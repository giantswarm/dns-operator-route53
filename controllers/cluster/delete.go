package cluster

import (
	"context"

	"github.com/giantswarm/microerror"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/giantswarm/dns-operator-openstack/pkg/cloud"
	"github.com/giantswarm/dns-operator-openstack/pkg/cloud/services/route53"
	"github.com/giantswarm/dns-operator-openstack/pkg/key"
)

func (r *Reconciler) reconcileDelete(ctx context.Context, clusterScope cloud.ClusterScoper) (reconcile.Result, error) {
	clusterScope.Info("reconciling delete")

	route53Service := route53.NewService(clusterScope)

	if err := route53Service.DeleteRoute53(ctx); err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	openstackCluster := clusterScope.InfrastructureCluster()
	if controllerutil.ContainsFinalizer(openstackCluster, key.DNSFinalizerName) {
		clusterScope.Info("removing finalizer")
		controllerutil.RemoveFinalizer(openstackCluster, key.DNSFinalizerName)
		if err := r.client.Update(ctx, openstackCluster); err != nil {
			return reconcile.Result{}, microerror.Mask(err)
		}
		clusterScope.Info("removed finalizer")
	}

	clusterScope.Info("reconciled delete")

	return ctrl.Result{}, nil
}
