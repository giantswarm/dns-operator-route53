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

package main

import (
	"flag"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/giantswarm/dns-operator-route53/controllers"

	dnscache "github.com/giantswarm/dns-operator-route53/pkg/cloud/cache"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = capi.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var (
		baseDomain           string
		enableLeaderElection bool
		managementCluster    string
		metricsAddr          string
		roleArn              string
		staticBastionIP      string
	)

	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")

	flag.StringVar(&baseDomain, "base-domain", "", "Domain for which to create the DNS entries, e.g. customer.gigantic.io.")
	flag.StringVar(&managementCluster, "management-cluster", "", "Name of the management cluster.")
	flag.StringVar(&roleArn, "role-arn", "", "ARN of the role to assume for the AWS API calls.")
	flag.StringVar(&staticBastionIP, "static-bastion-ip", "", "IP address of static bastion machine for all clusters.")

	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Initialize the cache with a custom configuration
	var err error
	dnscache.DNSOperatorCache, err = dnscache.NewDNSOperatorCache()
	if err != nil {
		setupLog.Error(err, "unable to create the cache")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		LeaderElection:   enableLeaderElection,
		LeaderElectionID: "dns-operator-route53.giantswarm.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.ClusterReconciler{
		Client:            mgr.GetClient(),
		BaseDomain:        baseDomain,
		ManagementCluster: managementCluster,
		RoleArn:           roleArn,
		StaticBastionIP:   staticBastionIP,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cluster")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
