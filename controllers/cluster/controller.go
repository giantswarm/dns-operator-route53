package cluster

import (
	"context"
	"fmt"

	"github.com/giantswarm/microerror"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	capo "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha4"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/giantswarm/dns-operator-openstack/pkg/cloud/scope"
)

const Name = "cluster"

func New(config Config) (*Reconciler, error) {
	if config.Client == nil {
		return nil, microerror.Maskf(invalidConfigError, fmt.Sprintf("%T.Client is required", config))
	}
	if config.Logger == nil {
		return nil, microerror.Maskf(invalidConfigError, fmt.Sprintf("%T.Logger is required", config))
	}
	if config.BaseDomain == "" {
		return nil, microerror.Maskf(invalidConfigError, fmt.Sprintf("%T.BaseDomain is required", config))
	}
	if config.ManagementCluster == "" {
		return nil, microerror.Maskf(invalidConfigError, fmt.Sprintf("%T.ManagementCluster is required", config))
	}

	return &Reconciler{
		awsSession: config.AWSSession,
		client:     config.Client,
		logger:     config.Logger,

		baseDomain:        config.BaseDomain,
		managementCluster: config.ManagementCluster,
	}, nil
}

func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capo.OpenStackCluster{}).
		Complete(r)
}

// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=openstackclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=openstackclusters/status,verbs=get;update;patch

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.logger.WithValues("OpenStackCluster", req.NamespacedName)

	var infraCluster capo.OpenStackCluster
	err := r.client.Get(ctx, req.NamespacedName, &infraCluster)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, microerror.Mask(err)
	}

	// Fetch the owner cluster.
	coreCluster, err := util.GetOwnerCluster(ctx, r.client, infraCluster.ObjectMeta)
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}
	if coreCluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef")
		return reconcile.Result{
			Requeue: true,
		}, nil
	}

	log = log.WithValues("Cluster", types.NamespacedName{
		Namespace: coreCluster.Namespace,
		Name:      coreCluster.Name,
	})

	// Return early if the core or infrastructure cluster is paused.
	if annotations.IsPaused(coreCluster, &infraCluster) {
		log.Info("infrastructure or core cluster is marked as paused. Won't reconcile")
		return ctrl.Result{}, nil
	}

	// Create the cluster scope.
	clusterScope, err := scope.NewClusterScope(ctx, scope.ClusterScopeParams{
		AWSSession:       r.awsSession,
		ManagementClient: r.client,
		Logger:           log,

		BaseDomain:            r.baseDomain,
		InfrastructureCluster: &infraCluster,
		ManagementCluster:     r.managementCluster,
	})
	if err != nil {
		return reconcile.Result{}, microerror.Mask(err)
	}

	if !infraCluster.DeletionTimestamp.IsZero() {
		// Handle deleted clusters
		return r.reconcileDelete(ctx, clusterScope)
	}

	// Handle non-deleted clusters
	return r.reconcileNormal(ctx, clusterScope)
}
