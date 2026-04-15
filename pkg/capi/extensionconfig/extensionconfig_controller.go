/*
Copyright 2022 The Kubernetes Authors.

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

// Package extensionconfig provides an ExtensionConfig reconciler.
//
// Copied from sigs.k8s.io/cluster-api/internal/controllers/extensionconfig,
// adapted for external consumption (using only public CAPI APIs).
// Remove when upstream CAPI exposes these APIs publicly via the controllers package.
package extensionconfig

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
	"sigs.k8s.io/cluster-api/util/conditions"
	v1beta1conditions "sigs.k8s.io/cluster-api/util/conditions/deprecated/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/paused"
	"sigs.k8s.io/cluster-api/util/predicates"
)

const (
	// tlsCAKey is used as a data key in Secret resources to store a CA certificate.
	tlsCAKey = "ca.crt"
)

// Reconciler reconciles an ExtensionConfig object.
type Reconciler struct {
	Client             client.Client
	APIReader          client.Reader
	RuntimeClient      runtimeclient.Client
	PartialSecretCache cache.Cache

	// ReadOnly configures if the ExtensionConfig controller should write ExtensionConfig objects or only read them.
	ReadOnly bool

	// WatchFilterValue is the label value used to filter events prior to reconciliation.
	WatchFilterValue string
}

// SetupWithManager sets up the controller with the Manager.
func (r *Reconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, options controller.Options) error {
	if r.Client == nil || r.APIReader == nil || r.RuntimeClient == nil {
		return errors.New("Client, APIReader and RuntimeClient must not be nil")
	}

	if r.ReadOnly && r.PartialSecretCache != nil {
		return errors.New("PartialSecretCache must not be set if ReadOnly is true")
	}

	if !r.ReadOnly && r.PartialSecretCache == nil {
		return errors.New("PartialSecretCache must be set if ReadOnly is false")
	}

	predicateLog := ctrl.LoggerFrom(ctx).WithValues("controller", "extensionconfig")
	b := ctrl.NewControllerManagedBy(mgr).
		For(&runtimev1.ExtensionConfig{}).
		WithOptions(options).
		WithEventFilter(predicates.ResourceHasFilterLabel(mgr.GetScheme(), predicateLog, r.WatchFilterValue))

	if !r.ReadOnly {
		b.WatchesRawSource(source.Kind(
			r.PartialSecretCache,
			&metav1.PartialObjectMetadata{
				TypeMeta: metav1.TypeMeta{
					Kind:       "Secret",
					APIVersion: "v1",
				},
			},
			handler.TypedEnqueueRequestsFromMapFunc(
				r.secretToExtensionConfig,
			),
			predicates.TypedResourceIsChanged[*metav1.PartialObjectMetadata](mgr.GetScheme(), predicateLog),
		))
	}

	if err := b.Complete(r); err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	if err := indexByExtensionInjectCAFromSecretName(ctx, mgr); err != nil {
		return errors.Wrap(err, "failed setting up with a controller manager")
	}

	// warmupRunnable will attempt to sync the RuntimeSDK registry with existing ExtensionConfig objects to ensure extensions
	// are discovered before controllers begin reconciling.
	err := mgr.Add(&warmupRunnable{
		Client:        r.Client,
		APIReader:     r.APIReader,
		RuntimeClient: r.RuntimeClient,
		ReadOnly:      r.ReadOnly,
	})
	if err != nil {
		return errors.Wrap(err, "failed adding warmupRunnable to controller manager")
	}

	return nil
}

// Reconcile reconciles an ExtensionConfig object.
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Requeue events when the registry is not ready.
	// The registry will become ready after it is 'warmed up' by warmupRunnable.
	if !r.RuntimeClient.IsReady() {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	extensionConfig := &runtimev1.ExtensionConfig{}

	err := r.Client.Get(ctx, req.NamespacedName, extensionConfig)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// ExtensionConfig not found. Remove from registry.
			extensionConfig.Name = req.Name
			extensionConfig.Namespace = req.Namespace

			return r.reconcileDelete(ctx, extensionConfig)
		}

		return ctrl.Result{}, err
	}

	// Handle deletion reconciliation loop.
	if !extensionConfig.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, extensionConfig)
	}

	// In readOnly mode only validate instead of reconciling CA bundle and running discovery.
	if r.ReadOnly {
		if conditions.IsTrue(extensionConfig, clusterv1.PausedCondition) {
			return ctrl.Result{}, nil
		}

		if err := validateExtensionConfig(extensionConfig); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to validate ExtensionConfig")
		}

		// Register the ExtensionConfig if it is valid.
		log.V(4).Info("Registering ExtensionConfig information into registry")

		if err = r.RuntimeClient.Register(extensionConfig); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to register ExtensionConfig %s/%s", extensionConfig.Namespace, extensionConfig.Name)
		}
	} else {
		// Preserve original, EnsurePausedCondition might bump observedGeneration of the Paused condition without requeuing.
		original := extensionConfig.DeepCopy()

		if isPaused, requeue, err := paused.EnsurePausedCondition(ctx, r.Client, nil, extensionConfig); err != nil || isPaused || requeue {
			return ctrl.Result{}, err
		}

		extensionConfig, err := reconcileExtensionConfig(ctx, r.Client, r.RuntimeClient, original, extensionConfig)
		if err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to reconcile ExtensionConfig")
		}

		// Register the ExtensionConfig if it was found and patched without error.
		log.V(4).Info("Registering ExtensionConfig information into registry")

		if err = r.RuntimeClient.Register(extensionConfig); err != nil {
			return ctrl.Result{}, errors.Wrapf(err, "failed to register ExtensionConfig %s/%s", extensionConfig.Namespace, extensionConfig.Name)
		}
	}

	return ctrl.Result{}, nil
}

func patchExtensionConfig(ctx context.Context, c client.Client, original, modified *runtimev1.ExtensionConfig, options ...patch.Option) error {
	patchHelper, err := patch.NewHelper(original, c)
	if err != nil {
		return err
	}

	options = append(options,
		patch.WithOwnedV1Beta1Conditions{Conditions: []clusterv1.ConditionType{
			runtimev1.RuntimeExtensionDiscoveredV1Beta1Condition,
		}},
		patch.WithOwnedConditions{Conditions: []string{
			clusterv1.PausedCondition,
			runtimev1.ExtensionConfigDiscoveredCondition,
		}},
	)

	return patchHelper.Patch(ctx, modified, options...)
}

func (r *Reconciler) reconcileDelete(ctx context.Context, extensionConfig *runtimev1.ExtensionConfig) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Unregistering ExtensionConfig information from registry")

	if err := r.RuntimeClient.Unregister(extensionConfig); err != nil {
		return ctrl.Result{}, errors.Wrapf(err, "failed to unregister ExtensionConfig %s", klog.KObj(extensionConfig))
	}

	return ctrl.Result{}, nil
}

func (r *Reconciler) secretToExtensionConfig(ctx context.Context, secret *metav1.PartialObjectMetadata) []reconcile.Request {
	result := []ctrl.Request{}

	extensionConfigs := runtimev1.ExtensionConfigList{}
	indexKey := secret.GetNamespace() + "/" + secret.GetName()

	if err := r.Client.List(
		ctx,
		&extensionConfigs,
		client.MatchingFields{injectCAFromSecretAnnotationField: indexKey},
	); err != nil {
		return nil
	}

	for _, ext := range extensionConfigs.Items {
		result = append(result, ctrl.Request{NamespacedName: client.ObjectKey{Name: ext.Name}})
	}

	return result
}

func discoverExtensionConfig(
	ctx context.Context,
	runtimeClient runtimeclient.Client,
	extensionConfig *runtimev1.ExtensionConfig,
) (*runtimev1.ExtensionConfig, error) {
	discoveredExtension, err := runtimeClient.Discover(ctx, extensionConfig.DeepCopy())
	if err != nil {
		modifiedExtensionConfig := extensionConfig.DeepCopy()
		v1beta1conditions.MarkFalse(modifiedExtensionConfig,
			runtimev1.RuntimeExtensionDiscoveredV1Beta1Condition,
			runtimev1.DiscoveryFailedV1Beta1Reason,
			clusterv1.ConditionSeverityError, "Error in discovery: %v", err)
		conditions.Set(modifiedExtensionConfig, metav1.Condition{
			Type:    runtimev1.ExtensionConfigDiscoveredCondition,
			Status:  metav1.ConditionFalse,
			Reason:  runtimev1.ExtensionConfigNotDiscoveredReason,
			Message: fmt.Sprintf("Error in discovery: %v", err),
		})

		return modifiedExtensionConfig, errors.Wrapf(err, "failed to discover ExtensionConfig %s", klog.KObj(extensionConfig))
	}

	v1beta1conditions.MarkTrue(discoveredExtension, runtimev1.RuntimeExtensionDiscoveredV1Beta1Condition)
	conditions.Set(discoveredExtension, metav1.Condition{
		Type:   runtimev1.ExtensionConfigDiscoveredCondition,
		Status: metav1.ConditionTrue,
		Reason: runtimev1.ExtensionConfigDiscoveredReason,
	})

	return discoveredExtension, nil
}

func reconcileCABundle(ctx context.Context, c client.Client, config *runtimev1.ExtensionConfig) error {
	log := ctrl.LoggerFrom(ctx)

	secretNameRaw, ok := config.Annotations[runtimev1.InjectCAFromSecretAnnotation]
	if !ok {
		return nil
	}

	secretName := splitNamespacedName(secretNameRaw)

	log.V(4).Info(fmt.Sprintf("Injecting CA Bundle into ExtensionConfig from secret %q", secretNameRaw))

	if secretName.Namespace == "" || secretName.Name == "" {
		return errors.Errorf("failed to reconcile caBundle: secret name %q must be in the form <namespace>/<name>", secretNameRaw)
	}

	var secret corev1.Secret
	if err := c.Get(ctx, secretName, &secret); err != nil {
		return errors.Wrapf(err, "failed to reconcile caBundle: failed to get secret %q", secretNameRaw)
	}

	caData, hasCAData := secret.Data[tlsCAKey]
	if !hasCAData {
		return errors.Errorf("failed to reconcile caBundle: secret %s does not contain a %q entry", secretNameRaw, tlsCAKey)
	}

	config.Spec.ClientConfig.CABundle = caData

	return nil
}

func splitNamespacedName(nameStr string) types.NamespacedName {
	splitPoint := strings.IndexRune(nameStr, types.Separator)
	if splitPoint == -1 {
		return types.NamespacedName{Name: nameStr}
	}

	return types.NamespacedName{Namespace: nameStr[:splitPoint], Name: nameStr[splitPoint+1:]}
}

func validateExtensionConfig(extensionConfig *runtimev1.ExtensionConfig) error {
	if len(extensionConfig.Spec.ClientConfig.CABundle) == 0 {
		return errors.Errorf("caBundle is not set on ExtensionConfig %s", klog.KObj(extensionConfig))
	}

	discoveredCondition := conditions.Get(extensionConfig, runtimev1.ExtensionConfigDiscoveredCondition)
	switch {
	case discoveredCondition == nil:
		return errors.Errorf("%s condition not yet set on ExtensionConfig %s", runtimev1.ExtensionConfigDiscoveredCondition, klog.KObj(extensionConfig))
	case discoveredCondition.Status != metav1.ConditionTrue:
		return errors.Errorf(
			"%s condition on ExtensionConfig %s must have status: True (instead it has: %s)",
			runtimev1.ExtensionConfigDiscoveredCondition, klog.KObj(extensionConfig), discoveredCondition.Status)
	case discoveredCondition.ObservedGeneration != extensionConfig.Generation:
		return errors.Errorf(
			"%s condition on ExtensionConfig %s must have observedGeneration: %d (instead it has: %d)",
			runtimev1.ExtensionConfigDiscoveredCondition, klog.KObj(extensionConfig),
			extensionConfig.Generation, discoveredCondition.ObservedGeneration)
	}

	return nil
}

func reconcileExtensionConfig(
	ctx context.Context,
	c client.Client,
	runtimeClient runtimeclient.Client,
	original, extensionConfig *runtimev1.ExtensionConfig,
) (*runtimev1.ExtensionConfig, error) {
	if err := reconcileCABundle(ctx, c, extensionConfig); err != nil {
		return nil, err
	}

	if !bytes.Equal(original.Spec.ClientConfig.CABundle, extensionConfig.Spec.ClientConfig.CABundle) {
		if err := c.Patch(ctx, extensionConfig, client.MergeFrom(original)); err != nil {
			return nil, errors.Wrapf(err, "failed to patch ExtensionConfig %s", klog.KObj(extensionConfig))
		}

		original = extensionConfig.DeepCopy()
	}

	var errs []error

	extensionConfig, err := discoverExtensionConfig(ctx, runtimeClient, extensionConfig)
	if err != nil {
		errs = append(errs, err)
	}

	if err := patchExtensionConfig(ctx, c, original, extensionConfig); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return nil, kerrors.NewAggregate(errs)
	}

	return extensionConfig, nil
}
