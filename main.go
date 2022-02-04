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
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/giantswarm/microerror"
	"github.com/giantswarm/micrologger"
	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	capo "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha4"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/giantswarm/dns-operator-openstack/controllers"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = capi.AddToScheme(scheme)
	_ = capo.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	err := mainE()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, microerror.Pretty(err, true))
		os.Exit(1)
	}
}

type loggerAdapter struct {
	micrologger.Logger
	verbosity int
	names     []string
}

func (m loggerAdapter) Enabled() bool {
	return m.verbosity > 0
}

func (m loggerAdapter) Info(msg string, keysAndValues ...interface{}) {
	if m.verbosity < 2 {
		return
	}
	m.withName().With(keysAndValues...).With("level", "info").Log("message", msg)
}

func (m loggerAdapter) Error(err error, msg string, keysAndValues ...interface{}) {
	if m.verbosity < 1 {
		return
	}
	m.withName().With(keysAndValues...).Errorf(context.Background(), err, msg)
}

func (m loggerAdapter) withName() loggerAdapter {
	wrapperCopy := m
	if len(m.names) == 0 {
		wrapperCopy.Logger = m.Logger.With("name", strings.Join(m.names, "."))
	}
	return wrapperCopy
}

func (m loggerAdapter) V(level int) logr.Logger {
	wrapperCopy := m
	wrapperCopy.verbosity = level
	return wrapperCopy
}

func (m loggerAdapter) WithValues(keysAndValues ...interface{}) logr.Logger {
	wrapperCopy := m
	wrapperCopy.Logger = m.Logger.With(keysAndValues...)
	return wrapperCopy
}

func (m loggerAdapter) WithName(name string) logr.Logger {
	wrapperCopy := m
	wrapperCopy.names = append(m.names[:], name)
	return wrapperCopy
}

func mainE() error {
	var (
		baseDomain           string
		enableLeaderElection bool
		managementCluster    string
		metricsAddr          string
		verbosity            int
	)

	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")

	flag.StringVar(&baseDomain, "base-domain", "", "Domain for which to create the DNS entries, e.g. customer.gigantic.io.")
	flag.StringVar(&managementCluster, "management-cluster", "", "Name of the management cluster.")
	flag.IntVar(&verbosity, "verbosity", 3, "Name of the management cluster.")

	flag.Parse()

	// Create a new logger which is used by all packages.
	logger, err := micrologger.New(micrologger.Config{})
	if err != nil {
		return microerror.Mask(err)
	}

	ctrl.SetLogger(loggerAdapter{Logger: logger})

	config, err := ctrl.GetConfig()
	if err != nil {
		return microerror.Mask(err)
	}

	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "dns-operator-openstack.giantswarm.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return microerror.Mask(err)
	}

	if err = (&controllers.OpenstackClusterReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("OpenstackCluster"),

		BaseDomain:        baseDomain,
		ManagementCluster: managementCluster,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OpenstackCluster")
		return microerror.Mask(err)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		return microerror.Mask(err)
	}

	return nil
}
