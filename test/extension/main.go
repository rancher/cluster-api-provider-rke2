/*
Copyright 2026 SUSE.

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
	"fmt"
	"os"
	goruntime "runtime"
	"time"

	_ "k8s.io/component-base/logs/json/register"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	cliflag "k8s.io/component-base/cli/flag"
	"k8s.io/component-base/logs"
	logsv1 "k8s.io/component-base/logs/api/v1"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	"sigs.k8s.io/cluster-api/controllers/remote"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	"sigs.k8s.io/cluster-api/exp/runtime/server"
	"sigs.k8s.io/cluster-api/util/flags"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
	"github.com/rancher/cluster-api-provider-rke2/test/extension/handlers/inplaceupdate"
)

// Flag defaults. These mirror the upstream CAPI test extension and Kubernetes
// component-base conventions; kept as named constants so the linter doesn't
// flag them as magic numbers.
const (
	defaultLeaderElectionLeaseDurationSeconds = 15
	defaultRestConfigQPS                      = 100
	defaultRestConfigBurst                    = 200
	defaultWebhookPort                        = 9443
)

var (
	catalog        = runtimecatalog.New()
	scheme         = runtime.NewScheme()
	setupLog       = ctrl.Log.WithName("setup")
	controllerName = "caprke2-test-extension-manager"

	enableLeaderElection        bool
	leaderElectionLeaseDuration time.Duration
	leaderElectionRenewDeadline time.Duration
	leaderElectionRetryPeriod   time.Duration
	profilerAddress             string
	enableContentionProfiling   bool
	syncPeriod                  time.Duration
	restConfigQPS               float32
	restConfigBurst             int
	webhookPort                 int
	webhookCertDir              string
	webhookCertName             string
	webhookKeyName              string
	healthAddr                  string
	managerOptions              = flags.ManagerOptions{}
	logOptions                  = logs.NewOptions()
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = bootstrapv1.AddToScheme(scheme)
	_ = controlplanev1.AddToScheme(scheme)

	_ = runtimehooksv1.AddToCatalog(catalog)
}

// InitFlags wires the command-line flags consumed by the test extension.
func InitFlags(fs *pflag.FlagSet) {
	logsv1.AddFlags(logOptions, fs)

	fs.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election. With a single replica this only affects startup ordering.")
	fs.DurationVar(&leaderElectionLeaseDuration, "leader-elect-lease-duration", defaultLeaderElectionLeaseDurationSeconds*time.Second,
		"Interval at which non-leader candidates will wait to force acquire leadership.")
	fs.DurationVar(&leaderElectionRenewDeadline, "leader-elect-renew-deadline", 10*time.Second,
		"Duration the leader will retry refreshing leadership before giving up.")
	fs.DurationVar(&leaderElectionRetryPeriod, "leader-elect-retry-period", 2*time.Second,
		"Duration between LeaderElector retries.")
	fs.StringVar(&profilerAddress, "profiler-address", "",
		"Bind address for the pprof profiler (e.g. localhost:6060).")
	fs.BoolVar(&enableContentionProfiling, "contention-profiling", false,
		"Enable block profiling.")
	fs.DurationVar(&syncPeriod, "sync-period", 10*time.Minute,
		"Minimum interval at which watched resources are reconciled.")
	fs.Float32Var(&restConfigQPS, "kube-api-qps", defaultRestConfigQPS,
		"Maximum QPS from the controller client to the API server.")
	fs.IntVar(&restConfigBurst, "kube-api-burst", defaultRestConfigBurst,
		"Maximum burst from the controller client to the API server.")
	fs.IntVar(&webhookPort, "webhook-port", defaultWebhookPort,
		"Webhook server port.")
	fs.StringVar(&webhookCertDir, "webhook-cert-dir", "/tmp/k8s-webhook-server/serving-certs/",
		"Webhook cert directory.")
	fs.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt",
		"Webhook cert filename.")
	fs.StringVar(&webhookKeyName, "webhook-key-name", "tls.key",
		"Webhook key filename.")
	fs.StringVar(&healthAddr, "health-addr", ":9440",
		"Address the health endpoint binds to.")

	flags.AddManagerOptions(fs, &managerOptions)
}

func main() {
	InitFlags(pflag.CommandLine)
	pflag.CommandLine.SetNormalizeFunc(cliflag.WordSepNormalizeFunc)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	if err := pflag.CommandLine.Set("v", "2"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to set default log level: %v\n", err)
		os.Exit(1)
	}

	pflag.Parse()

	if err := logsv1.ValidateAndApply(logOptions, nil); err != nil {
		fmt.Fprintf(os.Stderr, "Unable to start manager: %v\n", err)
		os.Exit(1)
	}

	pflag.CommandLine.VisitAll(func(flag *pflag.Flag) {
		klog.V(1).Infof("FLAG: --%s=%q", flag.Name, flag.Value)
	})

	ctrl.SetLogger(klog.Background())

	restConfig := ctrl.GetConfigOrDie()
	restConfig.QPS = restConfigQPS
	restConfig.Burst = restConfigBurst
	restConfig.UserAgent = remote.DefaultClusterAPIUserAgent(controllerName)

	tlsOptions, metricsOptions, err := flags.GetManagerOptions(managerOptions)
	if err != nil {
		setupLog.Error(err, "Unable to start manager: invalid flags")
		os.Exit(1)
	}

	if enableContentionProfiling {
		goruntime.SetBlockProfileRate(1)
	}

	runtimeExtensionWebhookServer, err := server.New(server.Options{
		Port:     webhookPort,
		CertDir:  webhookCertDir,
		CertName: webhookCertName,
		KeyName:  webhookKeyName,
		TLSOpts:  tlsOptions,
		Catalog:  catalog,
	})
	if err != nil {
		setupLog.Error(err, "Error creating runtime extension webhook server")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(restConfig, ctrl.Options{
		Scheme:                     scheme,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           "caprke2-test-extension-leader-election",
		LeaseDuration:              &leaderElectionLeaseDuration,
		RenewDeadline:              &leaderElectionRenewDeadline,
		RetryPeriod:                &leaderElectionRetryPeriod,
		LeaderElectionResourceLock: resourcelock.LeasesResourceLock,
		HealthProbeBindAddress:     healthAddr,
		PprofBindAddress:           profilerAddress,
		Metrics:                    *metricsOptions,
		Cache: cache.Options{
			SyncPeriod: &syncPeriod,
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
				},
				Unstructured: true,
			},
		},
		WebhookServer: runtimeExtensionWebhookServer,
	})
	if err != nil {
		setupLog.Error(err, "Unable to start manager")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()

	setupInPlaceUpdateHookHandlers(mgr, runtimeExtensionWebhookServer)
	setupChecks(mgr)

	setupLog.Info("Starting CAPRKE2 test extension manager")

	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "Problem running manager")
		os.Exit(1)
	}
}

func setupInPlaceUpdateHookHandlers(mgr ctrl.Manager, runtimeExtensionWebhookServer *server.Server) {
	h := inplaceupdate.NewExtensionHandlers(mgr.GetClient())

	if err := runtimeExtensionWebhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.CanUpdateMachine,
		Name:        "can-update-machine",
		HandlerFunc: h.DoCanUpdateMachine,
	}); err != nil {
		setupLog.Error(err, "Error adding CanUpdateMachine handler")
		os.Exit(1)
	}

	if err := runtimeExtensionWebhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.CanUpdateMachineSet,
		Name:        "can-update-machineset",
		HandlerFunc: h.DoCanUpdateMachineSet,
	}); err != nil {
		setupLog.Error(err, "Error adding CanUpdateMachineSet handler")
		os.Exit(1)
	}

	if err := runtimeExtensionWebhookServer.AddExtensionHandler(server.ExtensionHandler{
		Hook:        runtimehooksv1.UpdateMachine,
		Name:        "update-machine",
		HandlerFunc: h.DoUpdateMachine,
	}); err != nil {
		setupLog.Error(err, "Error adding UpdateMachine handler")
		os.Exit(1)
	}
}

func setupChecks(mgr ctrl.Manager) {
	if err := mgr.AddReadyzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "Unable to create ready check")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("webhook", mgr.GetWebhookServer().StartedChecker()); err != nil {
		setupLog.Error(err, "Unable to create health check")
		os.Exit(1)
	}
}
