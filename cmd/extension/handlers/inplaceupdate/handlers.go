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

package inplaceupdate

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"reflect"

	"github.com/pkg/errors"
	planapi "github.com/rancher/rancher/pkg/plan"
	planv1alpha1 "github.com/rancher/rancher/pkg/plan/api/plan.cattle.io/v1alpha1"
	"gomodules.xyz/jsonpatch/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"
	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"

	bootstrapv1 "github.com/rancher/cluster-api-provider-rke2/bootstrap/api/v1beta2"
	controlplanev1 "github.com/rancher/cluster-api-provider-rke2/controlplane/api/v1beta2"
)

// machinePlanSecretType is the Secret type used by Rancher/system-agent to carry machine plans.
// (Matches plan.SecretTypeMachinePlan in github.com/rancher/rancher/pkg/plan.)
const machinePlanSecretType = planapi.SecretTypeMachinePlan

// errMachinePlanSecretNotFound is returned by findMachinePlanSecret when no machine-plan Secret
// exists yet for the given Machine (Rancher creates it when system-agent first registers).
var errMachinePlanSecretNotFound = errors.New("machine-plan secret not found")

// errBootstrapConfigUnavailable is returned by decodeDesiredRKE2Config when no BootstrapConfig
// was provided, or it isn't an RKE2Config.
var errBootstrapConfigUnavailable = errors.New("no RKE2Config BootstrapConfig available")

// ExtensionHandlers provides a common struct shared across the in-place update hook handlers.
//
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;patch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
type ExtensionHandlers struct {
	decoder runtime.Decoder
	client  client.Client
}

// NewExtensionHandlers returns a new ExtensionHandlers for the in-place update hook handlers.
func NewExtensionHandlers(client client.Client) *ExtensionHandlers {
	scheme := runtime.NewScheme()
	_ = bootstrapv1.AddToScheme(scheme)
	_ = controlplanev1.AddToScheme(scheme)

	return &ExtensionHandlers{
		client: client,
		decoder: serializer.NewCodecFactory(scheme).UniversalDecoder(
			bootstrapv1.GroupVersion,
		),
	}
}

// canUpdateMachineSpec declares that this extension can update:
// * MachineSpec.Version.
func canUpdateMachineSpec(current, desired *clusterv1.MachineSpec) {
	if current.Version != desired.Version {
		current.Version = desired.Version
	}
}

// canUpdateRKE2ConfigSpec declares that this extension can update:
// * RKE2ConfigSpec.Files.
func canUpdateRKE2ConfigSpec(current, desired *bootstrapv1.RKE2ConfigSpec) {
	if !reflect.DeepEqual(current.Files, desired.Files) {
		current.Files = desired.Files
	}
}

// DoCanUpdateMachine implements the CanUpdateMachine hook.
func (h *ExtensionHandlers) DoCanUpdateMachine(
	ctx context.Context,
	req *runtimehooksv1.CanUpdateMachineRequest,
	resp *runtimehooksv1.CanUpdateMachineResponse,
) {
	log := ctrl.LoggerFrom(ctx).WithValues("Machine", klog.KObj(&req.Desired.Machine))
	log.Info("CanUpdateMachine is called")

	currentMachine, desiredMachine,
		currentBootstrapConfig, desiredBootstrapConfig, err := h.getObjectsFromCanUpdateMachineRequest(req)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()

		return
	}

	// Declare changes that this Runtime Extension can update in-place.

	// Machine
	canUpdateMachineSpec(&currentMachine.Spec, &desiredMachine.Spec)

	// BootstrapConfig (we can only update RKE2Configs)
	currentRKE2Config, isCurrentRKE2Config := currentBootstrapConfig.(*bootstrapv1.RKE2Config)

	desiredRKE2Config, isDesiredRKE2Config := desiredBootstrapConfig.(*bootstrapv1.RKE2Config)
	if isCurrentRKE2Config && isDesiredRKE2Config {
		canUpdateRKE2ConfigSpec(&currentRKE2Config.Spec, &desiredRKE2Config.Spec)
	}

	err = h.computeCanUpdateMachineResponse(req, resp, currentMachine, currentBootstrapConfig)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()

		return
	}

	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// DoCanUpdateMachineSet implements the CanUpdateMachineSet hook.
func (h *ExtensionHandlers) DoCanUpdateMachineSet(
	ctx context.Context,
	req *runtimehooksv1.CanUpdateMachineSetRequest,
	resp *runtimehooksv1.CanUpdateMachineSetResponse,
) {
	log := ctrl.LoggerFrom(ctx).WithValues("MachineSet", klog.KObj(&req.Desired.MachineSet))
	log.Info("CanUpdateMachineSet is called")

	currentMachineSet, desiredMachineSet,
		currentBootstrapConfigTemplate, desiredBootstrapConfigTemplate, err := h.getObjectsFromCanUpdateMachineSetRequest(req)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()

		return
	}

	// Declare changes that this Runtime Extension can update in-place.

	// MachineSet
	canUpdateMachineSpec(&currentMachineSet.Spec.Template.Spec, &desiredMachineSet.Spec.Template.Spec)

	// BootstrapConfig (we can only update RKE2ConfigTemplates)
	currentRKE2ConfigTemplate, isCurrentRKE2ConfigTemplate := currentBootstrapConfigTemplate.(*bootstrapv1.RKE2ConfigTemplate)

	desiredRKE2ConfigTemplate, isDesiredRKE2ConfigTemplate := desiredBootstrapConfigTemplate.(*bootstrapv1.RKE2ConfigTemplate)
	if isCurrentRKE2ConfigTemplate && isDesiredRKE2ConfigTemplate {
		canUpdateRKE2ConfigSpec(&currentRKE2ConfigTemplate.Spec.Template.Spec, &desiredRKE2ConfigTemplate.Spec.Template.Spec)
	}

	err = h.computeCanUpdateMachineSetResponse(req, resp, currentMachineSet, currentBootstrapConfigTemplate)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()

		return
	}

	resp.Status = runtimehooksv1.ResponseStatusSuccess
}

// retryAfterUpdateInProgressSeconds is how long CAPI waits before calling UpdateMachine again
// while an update is still in flight (waiting for system-agent registration or plan completion).
const retryAfterUpdateInProgressSeconds = 30

// DoUpdateMachine implements the UpdateMachine hook.
// It locates the machine-plan Secret for the given Machine (created by Rancher when system-agent
// registers), builds the upgrade Plan for the Machine's desired version, submits it to the Secret
// if not already submitted, and maps the plan's execution state back to a hook response.
func (h *ExtensionHandlers) DoUpdateMachine(
	ctx context.Context,
	req *runtimehooksv1.UpdateMachineRequest,
	resp *runtimehooksv1.UpdateMachineResponse,
) {
	log := ctrl.LoggerFrom(ctx).WithValues("Machine", klog.KObj(&req.Desired.Machine))
	log.Info("UpdateMachine is called")

	defer func() {
		log.Info("UpdateMachine response",
			"Machine", klog.KObj(&req.Desired.Machine),
			"status", resp.Status,
			"message", resp.Message,
			"retryAfterSeconds", resp.RetryAfterSeconds,
		)
	}()

	secret, err := h.findMachinePlanSecret(ctx, &req.Desired.Machine)
	if errors.Is(err, errMachinePlanSecretNotFound) {
		resp.Status = runtimehooksv1.ResponseStatusSuccess
		resp.Message = "Waiting for system-agent to register"
		resp.RetryAfterSeconds = retryAfterUpdateInProgressSeconds

		return
	}

	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()

		return
	}

	log.Info("machine-plan Secret found", "secret", klog.KObj(secret))

	desiredRKE2Config, err := h.decodeDesiredRKE2Config(req.Desired.BootstrapConfig)
	if err != nil && !errors.Is(err, errBootstrapConfigUnavailable) {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()

		return
	}

	var (
		desiredFiles []bootstrapv1.File
		agentConfig  bootstrapv1.RKE2AgentConfig
	)

	if desiredRKE2Config != nil {
		agentConfig = desiredRKE2Config.Spec.AgentConfig

		desiredFiles, err = h.resolveDesiredFiles(ctx, req.Desired.Machine.Namespace, desiredRKE2Config.Spec.Files)
		if err != nil {
			resp.Status = runtimehooksv1.ResponseStatusFailure
			resp.Message = err.Error()

			return
		}
	}

	desiredPlan, err := buildUpgradePlan(&req.Desired.Machine, desiredFiles, agentConfig)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = err.Error()

		return
	}

	planBytes, err := json.Marshal(desiredPlan)
	if err != nil {
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = errors.Wrap(err, "failed to marshal upgrade plan").Error()

		return
	}

	switch evaluatePlanOutcome(secret, planBytes) {
	case planOutcomeNotSubmitted:
		if err := writePlan(ctx, h.client, secret, planBytes); err != nil {
			resp.Status = runtimehooksv1.ResponseStatusFailure
			resp.Message = errors.Wrap(err, "failed to write upgrade plan to machine-plan Secret").Error()

			return
		}

		resp.Status = runtimehooksv1.ResponseStatusSuccess
		resp.Message = "Update plan submitted"
		resp.RetryAfterSeconds = retryAfterUpdateInProgressSeconds
	case planOutcomeWaiting:
		resp.Status = runtimehooksv1.ResponseStatusSuccess
		resp.Message = "Update in progress"
		resp.RetryAfterSeconds = retryAfterUpdateInProgressSeconds
	case planOutcomeSucceeded:
		resp.Status = runtimehooksv1.ResponseStatusSuccess
		resp.Message = "Update applied successfully"
	case planOutcomeFailed:
		resp.Status = runtimehooksv1.ResponseStatusFailure
		resp.Message = "system-agent failed to apply the update plan"
	}
}

// findMachinePlanSecret returns the machine-plan Secret for the given Machine, or nil if it has
// not yet been created by Rancher.
func (h *ExtensionHandlers) findMachinePlanSecret(ctx context.Context, machine *clusterv1.Machine) (*corev1.Secret, error) {
	secretList := &corev1.SecretList{}
	if err := h.client.List(ctx, secretList,
		client.InNamespace(machine.Namespace),
		client.MatchingLabels{
			planv1alpha1.MachineLifecycleGroupLabel: clusterv1.GroupVersion.Group,
			planv1alpha1.MachineLifecycleKindLabel:  "Machine",
			planv1alpha1.MachineLifecycleNameLabel:  machine.Name,
		},
	); err != nil {
		return nil, err
	}

	for i := range secretList.Items {
		if secretList.Items[i].Type == machinePlanSecretType {
			return &secretList.Items[i], nil
		}
	}

	return nil, errMachinePlanSecretNotFound
}

// decodeDesiredRKE2Config decodes the optional BootstrapConfig from an UpdateMachineRequest.
// It returns nil if no BootstrapConfig was provided or it isn't an RKE2Config.
func (h *ExtensionHandlers) decodeDesiredRKE2Config(bootstrapConfig runtime.RawExtension) (*bootstrapv1.RKE2Config, error) {
	if len(bootstrapConfig.Raw) == 0 {
		return nil, errBootstrapConfigUnavailable
	}

	decoded, _, err := h.decoder.Decode(bootstrapConfig.Raw, nil, bootstrapConfig.Object)
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode desired BootstrapConfig")
	}

	rke2Config, ok := decoded.(*bootstrapv1.RKE2Config)
	if !ok {
		return nil, errBootstrapConfigUnavailable
	}

	return rke2Config, nil
}

// resolveDesiredFiles returns files with any ContentFrom (Secret/ConfigMap-sourced content)
// resolved into inline, base64-encoded Content, so convertFiles never has to handle ContentFrom
// itself. Files without ContentFrom are returned unchanged.
func (h *ExtensionHandlers) resolveDesiredFiles(ctx context.Context, namespace string, files []bootstrapv1.File) ([]bootstrapv1.File, error) {
	if len(files) == 0 {
		return nil, nil
	}

	resolved := make([]bootstrapv1.File, len(files))

	for i, f := range files {
		if f.ContentFrom == nil {
			resolved[i] = f

			continue
		}

		content, err := h.resolveFileContentFrom(ctx, namespace, f.ContentFrom)
		if err != nil {
			return nil, errors.Wrapf(err, "file %s", f.Path)
		}

		f.Content = base64.StdEncoding.EncodeToString(content)
		f.Encoding = bootstrapv1.Base64
		f.ContentFrom = nil
		resolved[i] = f
	}

	return resolved, nil
}

// resolveFileContentFrom reads the referenced Secret or ConfigMap key, defaulting to namespace
// when the reference doesn't specify one.
func (h *ExtensionHandlers) resolveFileContentFrom(ctx context.Context, namespace string, ref *bootstrapv1.FileSource) ([]byte, error) {
	switch {
	case ref.Secret != nil:
		ns := ref.Secret.Namespace
		if ns == "" {
			ns = namespace
		}

		secret := &corev1.Secret{}
		if err := h.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: ref.Secret.Name}, secret); err != nil {
			return nil, errors.Wrapf(err, "failed to get secret %s/%s", ns, ref.Secret.Name)
		}

		data, ok := secret.Data[ref.Secret.Key]
		if !ok {
			return nil, errors.Errorf("key %q not found in secret %s/%s", ref.Secret.Key, ns, ref.Secret.Name)
		}

		return data, nil
	case ref.ConfigMap != nil:
		ns := ref.ConfigMap.Namespace
		if ns == "" {
			ns = namespace
		}

		cm := &corev1.ConfigMap{}
		if err := h.client.Get(ctx, client.ObjectKey{Namespace: ns, Name: ref.ConfigMap.Name}, cm); err != nil {
			return nil, errors.Wrapf(err, "failed to get configmap %s/%s", ns, ref.ConfigMap.Name)
		}

		if v, ok := cm.Data[ref.ConfigMap.Key]; ok {
			return []byte(v), nil
		}

		if v, ok := cm.BinaryData[ref.ConfigMap.Key]; ok {
			return v, nil
		}

		return nil, errors.Errorf("key %q not found in configmap %s/%s", ref.ConfigMap.Key, ns, ref.ConfigMap.Name)
	default:
		return nil, errors.New("contentFrom has neither secret nor configMap set")
	}
}

//nolint:dupl // mirrors getObjectsFromCanUpdateMachineSetRequest by design: same shape, different request type.
func (h *ExtensionHandlers) getObjectsFromCanUpdateMachineRequest(
	req *runtimehooksv1.CanUpdateMachineRequest,
) (
	*clusterv1.Machine,
	*clusterv1.Machine,
	runtime.Object,
	runtime.Object,
	error,
) {
	currentMachine := req.Current.Machine.DeepCopy()
	desiredMachine := req.Desired.Machine.DeepCopy()

	currentBootstrapConfig, _, err := h.decoder.Decode(req.Current.BootstrapConfig.Raw, nil, req.Current.BootstrapConfig.Object)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	desiredBootstrapConfig, _, err := h.decoder.Decode(req.Desired.BootstrapConfig.Raw, nil, req.Desired.BootstrapConfig.Object)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return currentMachine, desiredMachine, currentBootstrapConfig, desiredBootstrapConfig, nil
}

//nolint:dupl // mirrors computeCanUpdateMachineSetResponse by design: same shape, different request type.
func (h *ExtensionHandlers) computeCanUpdateMachineResponse(
	req *runtimehooksv1.CanUpdateMachineRequest,
	resp *runtimehooksv1.CanUpdateMachineResponse,
	currentMachine *clusterv1.Machine,
	currentBootstrapConfig runtime.Object,
) error {
	marshalledCurrentMachine, err := json.Marshal(req.Current.Machine)
	if err != nil {
		return err
	}

	machinePatch, err := createJSONPatch(marshalledCurrentMachine, currentMachine)
	if err != nil {
		return err
	}

	bootstrapConfigPatch, err := createJSONPatch(req.Current.BootstrapConfig.Raw, currentBootstrapConfig)
	if err != nil {
		return err
	}

	resp.MachinePatch = runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     machinePatch,
	}
	resp.BootstrapConfigPatch = runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     bootstrapConfigPatch,
	}

	return nil
}

//nolint:dupl // mirrors getObjectsFromCanUpdateMachineRequest by design: same shape, different request type.
func (h *ExtensionHandlers) getObjectsFromCanUpdateMachineSetRequest(
	req *runtimehooksv1.CanUpdateMachineSetRequest,
) (
	*clusterv1.MachineSet,
	*clusterv1.MachineSet,
	runtime.Object,
	runtime.Object,
	error,
) {
	currentMachineSet := req.Current.MachineSet.DeepCopy()
	desiredMachineSet := req.Desired.MachineSet.DeepCopy()

	currentBootstrapConfigTemplate, _, err := h.decoder.Decode(
		req.Current.BootstrapConfigTemplate.Raw, nil, req.Current.BootstrapConfigTemplate.Object)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	desiredBootstrapConfigTemplate, _, err := h.decoder.Decode(
		req.Desired.BootstrapConfigTemplate.Raw, nil, req.Desired.BootstrapConfigTemplate.Object)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	return currentMachineSet, desiredMachineSet,
		currentBootstrapConfigTemplate, desiredBootstrapConfigTemplate,
		nil
}

//nolint:dupl // mirrors computeCanUpdateMachineResponse by design: same shape, different request type.
func (h *ExtensionHandlers) computeCanUpdateMachineSetResponse(
	req *runtimehooksv1.CanUpdateMachineSetRequest,
	resp *runtimehooksv1.CanUpdateMachineSetResponse,
	currentMachineSet *clusterv1.MachineSet,
	currentBootstrapConfigTemplate runtime.Object,
) error {
	marshalledCurrentMachineSet, err := json.Marshal(req.Current.MachineSet)
	if err != nil {
		return err
	}

	machineSetPatch, err := createJSONPatch(marshalledCurrentMachineSet, currentMachineSet)
	if err != nil {
		return err
	}

	bootstrapConfigTemplatePatch, err := createJSONPatch(req.Current.BootstrapConfigTemplate.Raw, currentBootstrapConfigTemplate)
	if err != nil {
		return err
	}

	resp.MachineSetPatch = runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     machineSetPatch,
	}
	resp.BootstrapConfigTemplatePatch = runtimehooksv1.Patch{
		PatchType: runtimehooksv1.JSONPatchType,
		Patch:     bootstrapConfigTemplatePatch,
	}

	return nil
}

// createJSONPatch creates a RFC 6902 JSON patch from the original and the modified object.
func createJSONPatch(marshalledOriginal []byte, modified runtime.Object) ([]byte, error) {
	marshalledModified, err := json.Marshal(modified)
	if err != nil {
		return nil, errors.Errorf("failed to marshal modified object: %v", err)
	}

	patch, err := jsonpatch.CreatePatch(marshalledOriginal, marshalledModified)
	if err != nil {
		return nil, errors.Errorf("failed to create patch: %v", err)
	}

	patchBytes, err := json.Marshal(patch)
	if err != nil {
		return nil, errors.Errorf("failed to marshal patch: %v", err)
	}

	return patchBytes, nil
}
