package helpers

import (
	"context"
	"go/build"
	"os"
	"path"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strings"

	"github.com/onsi/ginkgo/v2"
	admissionv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	klog "k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	logf "sigs.k8s.io/cluster-api/cmd/clusterctl/log"
)

var (
	root                   string
	clusterAPIVersionRegex = regexp.MustCompile(`^(\W)sigs.k8s.io/cluster-api v(.+)`)
)

func init() {
	klog.InitFlags(nil)
	// additionally force all the controllers to use the Ginkgo logger.
	ctrl.SetLogger(klog.Background())
	logf.SetLogger(klog.Background())
	// add logger for ginkgo
	klog.SetOutput(ginkgo.GinkgoWriter)

	// Calculate the scheme.
	utilruntime.Must(apiextensionsv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(admissionv1.AddToScheme(scheme.Scheme))
	utilruntime.Must(clusterv1.AddToScheme(scheme.Scheme))

	// Get the root of the current file to use in CRD paths.
	_, filename, _, _ := goruntime.Caller(0) //nolint
	root = path.Join(path.Dir(filename), "..", "..")
}

// TestEnvironmentConfiguration is a wrapper configuration for envtest.
type TestEnvironmentConfiguration struct {
	env *envtest.Environment
}

// TestEnvironment encapsulates a Kubernetes local test environment.
type TestEnvironment struct {
	manager.Manager
	client.Client

	Config *rest.Config
	env    *envtest.Environment
	cancel context.CancelFunc
}

// Cleanup deletes all the given objects.
func (t *TestEnvironment) Cleanup(ctx context.Context, objs ...client.Object) error {
	errs := []error{}

	for _, o := range objs {
		err := t.Delete(ctx, o)
		if apierrors.IsNotFound(err) {
			continue
		}

		errs = append(errs, err)
	}

	return kerrors.NewAggregate(errs)
}

// CreateNamespace creates a new namespace with a generated name.
func (t *TestEnvironment) CreateNamespace(ctx context.Context, generateName string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: generateName + "-",
			Labels: map[string]string{
				"testenv/original-name": generateName,
			},
		},
	}
	if err := t.Create(ctx, ns); err != nil {
		return nil, err
	}

	return ns, nil
}

// NewTestEnvironmentConfiguration creates a new test environment configuration for running tests.
func NewTestEnvironmentConfiguration(crdDirectoryPaths []string) *TestEnvironmentConfiguration {
	resolvedCrdDirectoryPaths := []string{}

	for _, p := range crdDirectoryPaths {
		resolvedCrdDirectoryPaths = append(resolvedCrdDirectoryPaths, path.Join(root, p))
	}

	if capiPath := getFilePathToCAPICRDs(root); capiPath != "" {
		resolvedCrdDirectoryPaths = append(resolvedCrdDirectoryPaths, capiPath)
	}

	return &TestEnvironmentConfiguration{
		env: &envtest.Environment{
			ErrorIfCRDPathMissing: true,
			CRDDirectoryPaths:     resolvedCrdDirectoryPaths,
		},
	}
}

// Build creates a new environment spinning up a local api-server.
// This function should be called only once for each package you're running tests within,
// usually the environment is initialized in a suite_test.go file within a `BeforeSuite` ginkgo block.
func (t *TestEnvironmentConfiguration) Build() (*TestEnvironment, error) {
	if _, err := t.env.Start(); err != nil {
		panic(err)
	}

	options := manager.Options{
		Scheme: scheme.Scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		Client: client.Options{
			Cache: &client.CacheOptions{
				DisableFor: []client.Object{
					&corev1.ConfigMap{},
					&corev1.Secret{},
					&corev1.Node{},
					&clusterv1.Machine{},
				},
			},
		},
	}

	mgr, err := ctrl.NewManager(t.env.Config, options)
	if err != nil {
		klog.Fatalf("Failed to start testenv manager: %v", err)
	}

	return &TestEnvironment{
		Manager: mgr,
		Client:  mgr.GetClient(),
		Config:  mgr.GetConfig(),
		env:     t.env,
	}, nil
}

// StartManager starts the test controller against the local API server.
func (t *TestEnvironment) StartManager(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	t.cancel = cancel

	return t.Start(ctx)
}

// Stop stops the test environment.
func (t *TestEnvironment) Stop() error {
	t.cancel()

	return t.env.Stop()
}

func getFilePathToCAPICRDs(root string) string {
	modBits, err := os.ReadFile(filepath.Join(root, "go.mod")) //nolint:gosec
	if err != nil {
		return ""
	}

	var clusterAPIVersion string

	for _, line := range strings.Split(string(modBits), "\n") {
		matches := clusterAPIVersionRegex.FindStringSubmatch(line)
		if len(matches) == 3 {
			clusterAPIVersion = matches[2]
		}
	}

	if clusterAPIVersion == "" {
		return ""
	}

	gopath := envOr("GOPATH", build.Default.GOPATH)

	return filepath.Join(gopath, "pkg", "mod", "sigs.k8s.io", "cluster-api@v"+clusterAPIVersion, "config", "crd", "bases")
}

func envOr(envKey, defaultValue string) string {
	if value, ok := os.LookupEnv(envKey); ok {
		return value
	}

	return defaultValue
}
