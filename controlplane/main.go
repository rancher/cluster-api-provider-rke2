/*
Copyright 2022 SUSE.

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
	"time"

	_ "k8s.io/client-go/plugin/pkg/client/auth"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	klog "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	"sigs.k8s.io/cluster-api/util/flags"

	bootstrapv1beta1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta1"
	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1beta1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta1"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/controlplane/internal/controllers"
	"github.com/rancher/cluster-api-provider-rke2/pkg/consts"
	"github.com/rancher/cluster-api-provider-rke2/version"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")

	// flags.
	enableLeaderElection           bool
	leaderElectionLeaseDuration    time.Duration
	leaderElectionRenewDeadline    time.Duration
	leaderElectionRetryPeriod      time.Duration
	watchFilterValue               string
	profilerAddress                string
	concurrencyNumber              int
	syncPeriod                     time.Duration
	clusterCacheTrackerClientQPS   float32
	clusterCacheTrackerClientBurst int
	clusterCacheConcurrencyNumber  int
	watchNamespace                 string
	webhookPort                    int
	webhookCertDir                 string
	healthAddr                     string
	managerOptions                 = flags.ManagerOptions{}
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(controlplanev1.AddToScheme(scheme))
	utilruntime.Must(controlplanev1beta1.AddToScheme(scheme))
	utilruntime.Must(bootstrapv1.AddToScheme(scheme))
	utilruntime.Must(bootstrapv1beta1.AddToScheme(scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
} //nolint:wsl

// InitFlags initializes the flags.
func InitFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")

	fs.DurationVar(&leaderElectionLeaseDuration, "leader-elect-lease-duration", consts.DefaultLeaderElectLeaseDuration,
		"Interval at which non-leader candidates will wait to force acquire leadership (duration string)")

	fs.DurationVar(&leaderElectionRenewDeadline, "leader-elect-renew-deadline", consts.DefaultLeaderElectRenewDeadline,
		"Duration that the leading controller manager will retry refreshing leadership before giving up (duration string)")

	fs.DurationVar(&leaderElectionRetryPeriod, "leader-elect-retry-period", consts.DefaultLeaderElectRetryPeriod,
		"Duration the LeaderElector clients should wait between tries of actions (duration string)")

	fs.StringVar(&watchFilterValue, "watch-filter", "",
		fmt.Sprintf("Label value that the controller watches to reconcile cluster-api objects. Label key is always %s. If unspecified, the controller watches for all cluster-api objects.", clusterv1.WatchLabel)) //nolint:lll

	fs.StringVar(&profilerAddress, "profiler-address", "",
		"Bind address to expose the pprof profiler (e.g. localhost:6060)")

	fs.IntVar(&concurrencyNumber, "concurrency", 10,
		"Number of core resources to process simultaneously")

	fs.DurationVar(&syncPeriod, "sync-period", consts.DefaultSyncPeriod,
		"The minimum interval at which watched resources are reconciled (e.g. 15m)")

	fs.Float32Var(&clusterCacheTrackerClientQPS, "clustercachetracker-client-qps", 20,
		"Maximum queries per second from the cluster cache tracker clients to the Kubernetes API server of workload clusters.")

	fs.IntVar(&clusterCacheTrackerClientBurst, "clustercachetracker-client-burst", 30,
		"Maximum number of queries allowed in one burst from cluster cache tracker clients "+
			"to the Kubernetes API server of workload clusters.")

	fs.IntVar(&clusterCacheConcurrencyNumber, "clustercache-concurrency", consts.DefaultClusterCacheConcurrency,
		"Number of clusters to process simultaneously")

	fs.StringVar(&watchNamespace, "namespace", "",
		"Namespace that the controller watches to reconcile cluster-api objects. If unspecified, the controller watches for cluster-api objects across all namespaces.") //nolint:lll

	fs.IntVar(&webhookPort, "webhook-port", consts.DefaultWebhookPort, "Webhook Server port")

	fs.StringVar(&webhookCertDir, "webhook-cert-dir", "/tmp/k8s-webhook-server/serving-certs/",
		"Webhook cert dir, only used when webhook-port is specified.")

	fs.StringVar(&healthAddr, "health-addr", ":9440",
		"The address the health endpoint binds to.")

	flags.AddManagerOptions(fs, &managerOptions)
}

// Add RBAC for the authorized diagnostics endpoint.
// +kubebuilder:rbac:groups=authentication.k8s.io,resources=tokenreviews,verbs=create
// +kubebuilder:rbac:groups=authorization.k8s.io,resources=subjectaccessreviews,verbs=create

func main() {
	klog.InitFlags(nil)
	InitFlags(pflag.CommandLine)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()

	ctrl.SetLogger(klog.Background())

	restConfig := ctrl.GetConfigOrDie()

	tlsOptions, metricsOptions, err := flags.GetManagerOptions(managerOptions)
	if err != nil {
		setupLog.Error(err, "Unable to start manager: invalid flags")
		os.Exit(1)
	}

	var watchNamespaces map[string]cache.Config

	if watchNamespace != "" {
		setupLog.Info("Watching cluster-api objects only in namespace for reconciliation", "namespace", watchNamespace)
		watchNamespaces = map[string]cache.Config{
			watchNamespace: {},
		}
	}

	ctrlOptions := ctrl.Options{
		Scheme:                 scheme,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "rke2-controlplane-manager-leader-election-capi",
		PprofBindAddress:       profilerAddress,
		LeaseDuration:          &leaderElectionLeaseDuration,
		RenewDeadline:          &leaderElectionRenewDeadline,
		RetryPeriod:            &leaderElectionRetryPeriod,
		HealthProbeBindAddress: healthAddr,
		Metrics:                *metricsOptions,
		Cache: cache.Options{
			DefaultNamespaces: watchNamespaces,
			SyncPeriod:        &syncPeriod,
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
			},
		},
		WebhookServer: webhook.NewServer(
			webhook.Options{
				Port:    webhookPort,
				CertDir: webhookCertDir,
				TLSOpts: tlsOptions,
			},
		),
	}

	mgr, err := ctrl.NewManager(restConfig, ctrlOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Setup the context that's going to be used in controllers and for the manager.
	ctx := ctrl.SetupSignalHandler()

	setupChecks(mgr)
	setupWebhooks(mgr)
	setupReconcilers(ctx, mgr)

	setupLog.Info("Starting manager", "version", version.Get().String(), "concurrency", concurrencyNumber)

	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func setupChecks(mgr ctrl.Manager) {
	if err := mgr.AddReadyzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "unable to create ready check")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "unable to create health check")
		os.Exit(1)
	}
}

func setupReconcilers(ctx context.Context, mgr ctrl.Manager) {
	secretCachingClient, err := client.New(mgr.GetConfig(), client.Options{
		HTTPClient: mgr.GetHTTPClient(),
		Cache: &client.CacheOptions{
			Reader: mgr.GetCache(),
		},
	})
	if err != nil {
		setupLog.Error(err, "unable to create secret caching client")
		os.Exit(1)
	}

	if err := (&controllers.RKE2ControlPlaneReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		WatchFilterValue:    watchFilterValue,
		SecretCachingClient: secretCachingClient,
	}).SetupWithManager(
		ctx, mgr, clusterCacheTrackerClientQPS, clusterCacheTrackerClientBurst,
		clusterCacheConcurrencyNumber, concurrencyNumber,
	); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "RKE2ControlPlane")
		os.Exit(1)
	}
}

func setupWebhooks(mgr ctrl.Manager) {
	// Setup the func to retrieve apiVersion for a GroupKind for conversion webhooks.
	apiVersionGetter := func(gk schema.GroupKind) (string, error) {
		// CAPI's util function for fetching apiVersions `contract.GetAPIVersion` is now internal,
		// so we use a RESTMapper instead.
		mapping, err := mgr.GetRESTMapper().RESTMapping(gk)
		if err != nil {
			return "", err
		}

		return mapping.GroupVersionKind.GroupVersion().String(), nil
	}
	controlplanev1beta1.SetAPIVersionGetter(apiVersionGetter)

	if err := controlplanev1.SetupRKE2ControlPlaneTemplateWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "RKE2ControlPlaneTemplate")
		os.Exit(1)
	}

	if err := controlplanev1.SetupRKE2ControlPlaneWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "RKE2ControlPlane")
		os.Exit(1)
	}
}
