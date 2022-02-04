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

func (r *Reconciler) reconcileDelete(ctx context.Context, clusterScope cloud.ClusterScoper) (reconcile.Result, error) {
	clusterScope.Info("Reconciling delete")

	route53Service := route53.NewService(clusterScope)

	if err := route53Service.DeleteRoute53(ctx); err != nil {
		clusterScope.Error(err, "error deleting route53")
		return reconcile.Result{}, microerror.Mask(err)
	}

	{
		// Resources are deleted so remove the finalizer.
		openstackCluster := clusterScope.InfrastructureCluster()
		controllerutil.RemoveFinalizer(openstackCluster, key.DNSFinalizerName)
		if err := r.client.Update(ctx, openstackCluster); err != nil {
			return reconcile.Result{}, microerror.Mask(err)
		}
	}

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Minute * 5,
	}, nil
}
