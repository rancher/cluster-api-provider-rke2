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

// Package runtimeclient provides a Runtime SDK client factory.
//
// Copied from sigs.k8s.io/cluster-api/internal/runtime/client.
// Remove when upstream CAPI exposes these APIs publicly.
package runtimeclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net"
	"net/http"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	utilnet "k8s.io/apimachinery/pkg/util/net"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/transport"
	"k8s.io/klog/v2"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	ctrlmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"

	runtimehooksv1 "sigs.k8s.io/cluster-api/api/runtime/hooks/v1alpha1"
	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	runtimecatalog "sigs.k8s.io/cluster-api/exp/runtime/catalog"
	runtimeclientapi "sigs.k8s.io/cluster-api/exp/runtime/client"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/cache"

	capiregistry "github.com/rancher/cluster-api-provider-rke2/pkg/capi/registry"
	runtimemetrics "github.com/rancher/cluster-api-provider-rke2/pkg/capi/runtimemetrics"
)

// runtimeSDKSubsystem is the metrics subsystem.
const runtimeSDKSubsystem = "caprke2_runtime_sdk"

// defaultMetrics are Runtime SDK metrics registered with the controller-runtime registry.
var defaultMetrics = runtimemetrics.Register(ctrlmetrics.Registry, runtimeSDKSubsystem)

type errCallingExtensionHandler error

const (
	defaultDiscoveryTimeout   = 10 * time.Second
	defaultHTTPClientCacheTTL = 24 * time.Hour
)

// Options are creation options for a runtime Client.
type Options struct {
	CertFile string // Path of the PEM-encoded client certificate.
	KeyFile  string // Path of the PEM-encoded client key.
	Catalog  *runtimecatalog.Catalog
	Registry capiregistry.ExtensionRegistry
	Client   ctrlclient.Client
}

// New returns a new runtime Client.
func New(options Options) (runtimeclientapi.Client, *certwatcher.CertWatcher, error) {
	httpClientCache := cache.New[httpClientEntry](defaultHTTPClientCacheTTL)

	var certWatcher *certwatcher.CertWatcher

	if options.CertFile != "" && options.KeyFile != "" {
		var err error

		certWatcher, err = certwatcher.New(options.CertFile, options.KeyFile)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "failed to create RuntimeSDK client: failed to create cert-watcher")
		}

		certWatcher.RegisterCallback(func(_ tls.Certificate) {
			httpClientCache.DeleteAll()
		})
	}

	return &client{
		certFile:         options.CertFile,
		keyFile:          options.KeyFile,
		catalog:          options.Catalog,
		registry:         options.Registry,
		client:           options.Client,
		httpClientsCache: httpClientCache,
	}, certWatcher, nil
}

var _ runtimeclientapi.Client = &client{}

type client struct {
	certFile         string
	keyFile          string
	catalog          *runtimecatalog.Catalog
	registry         capiregistry.ExtensionRegistry
	client           ctrlclient.Client
	httpClientsCache cache.Cache[httpClientEntry]
}

type httpClientEntry struct {
	caData   []byte
	hostName string
	client   *http.Client
}

func newHTTPClientEntry(hostName string, caData []byte, c *http.Client) httpClientEntry {
	return httpClientEntry{
		hostName: hostName,
		caData:   caData,
		client:   c,
	}
}

func newHTTPClientEntryKey(hostName string, caData []byte) string {
	return httpClientEntry{
		hostName: hostName,
		caData:   caData,
	}.Key()
}

func (r httpClientEntry) Key() string {
	return fmt.Sprintf("%s/%s", r.hostName, string(r.caData))
}

func (c *client) WarmUp(extensionConfigList *runtimev1.ExtensionConfigList) error {
	return c.registry.WarmUp(extensionConfigList)
}

func (c *client) IsReady() bool {
	return c.registry.IsReady()
}

func (c *client) Discover(ctx context.Context, extensionConfig *runtimev1.ExtensionConfig) (*runtimev1.ExtensionConfig, error) {
	log := ctrl.LoggerFrom(ctx)
	log.V(4).Info("Performing discovery for ExtensionConfig")

	hookGVH, err := c.catalog.GroupVersionHook(runtimehooksv1.Discovery)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to discover extension %q: failed to compute GVH of hook", extensionConfig.Name)
	}

	httpClient, err := c.getHTTPClient(extensionConfig.Spec.ClientConfig)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to discover extension %q: failed to get http client", extensionConfig.Name)
	}

	request := &runtimehooksv1.DiscoveryRequest{}
	response := &runtimehooksv1.DiscoveryResponse{}

	opts := &httpCallOptions{
		catalog:         c.catalog,
		config:          extensionConfig.Spec.ClientConfig,
		registrationGVH: hookGVH,
		hookGVH:         hookGVH,
		timeout:         defaultDiscoveryTimeout,
		httpClient:      httpClient,
	}
	if err := httpCall(ctx, request, response, opts); err != nil {
		return nil, errors.Wrapf(err, "failed to discover extension %q", extensionConfig.Name)
	}

	if err := validateResponseStatus(log, response, "discover extension", extensionConfig.Name); err != nil {
		return nil, err
	}

	if err = defaultAndValidateDiscoveryResponse(c.catalog, response); err != nil {
		return nil, errors.Wrapf(err, "failed to discover extension %q", extensionConfig.Name)
	}

	modifiedExtensionConfig := extensionConfig.DeepCopy()
	modifiedExtensionConfig.Status.Handlers = []runtimev1.ExtensionHandler{}

	for _, handler := range response.Handlers {
		handlerName, err := nameForHandler(handler, extensionConfig)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to discover extension %q", extensionConfig.Name)
		}

		modifiedExtensionConfig.Status.Handlers = append(
			modifiedExtensionConfig.Status.Handlers,
			runtimev1.ExtensionHandler{
				Name: handlerName,
				RequestHook: runtimev1.GroupVersionHook{
					APIVersion: handler.RequestHook.APIVersion,
					Hook:       handler.RequestHook.Hook,
				},
				TimeoutSeconds: ptr.Deref(handler.TimeoutSeconds, 0),
				FailurePolicy:  runtimev1.FailurePolicy(ptr.Deref(handler.FailurePolicy, "")),
			},
		)
	}

	return modifiedExtensionConfig, nil
}

func (c *client) Register(extensionConfig *runtimev1.ExtensionConfig) error {
	if err := c.registry.Add(extensionConfig); err != nil {
		return errors.Wrapf(err, "failed to register ExtensionConfig %q", extensionConfig.Name)
	}

	return nil
}

func (c *client) Unregister(extensionConfig *runtimev1.ExtensionConfig) error {
	if err := c.registry.Remove(extensionConfig); err != nil {
		return errors.Wrapf(err, "failed to unregister ExtensionConfig %q", extensionConfig.Name)
	}

	return nil
}

func (c *client) GetAllExtensions(ctx context.Context, hook runtimecatalog.Hook, forObject ctrlclient.Object) ([]string, error) {
	hookName := runtimecatalog.HookName(hook)
	log := ctrl.LoggerFrom(ctx).WithValues("hook", hookName)
	ctx = ctrl.LoggerInto(ctx, log)

	gvh, err := c.catalog.GroupVersionHook(hook)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get extension handlers for hook %q: failed to compute GroupVersionHook", hookName)
	}

	forObjectGVK, err := apiutil.GVKForObject(forObject, c.client.Scheme())
	if err != nil {
		return nil, errors.Wrapf(err,
			"failed to get extension handlers for hook %q: failed to get GroupVersionKind for the object the hook is executed for",
			hookName)
	}

	registrations, err := c.registry.List(gvh.GroupHook())
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get extension handlers for hook %q", gvh.GroupHook())
	}

	log.V(4).Info(fmt.Sprintf("Getting all extensions of hook %q for %s %s", hookName, forObjectGVK.Kind, klog.KObj(forObject)))

	matchingRegistrations := []string{}

	for _, registration := range registrations {
		namespaceMatches, err := c.matchNamespace(ctx, registration.NamespaceSelector, forObject.GetNamespace())
		if err != nil {
			return nil, errors.Wrapf(err,
				"failed to get extension handlers for hook %q: failed to get extension handler %q",
				gvh.GroupHook(), registration.Name)
		}

		if !namespaceMatches {
			log.V(5).Info(fmt.Sprintf("skipping extension handler %q as object '%s/%s' does not match selector %q of ExtensionConfig",
				registration.Name, forObject.GetNamespace(), forObject.GetName(), registration.NamespaceSelector))

			continue
		}

		matchingRegistrations = append(matchingRegistrations, registration.Name)
	}

	return matchingRegistrations, nil
}

func (c *client) CallAllExtensions(
	ctx context.Context,
	hook runtimecatalog.Hook,
	forObject ctrlclient.Object,
	request runtimehooksv1.RequestObject,
	response runtimehooksv1.ResponseObject,
) error {
	hookName := runtimecatalog.HookName(hook)
	log := ctrl.LoggerFrom(ctx).WithValues("hook", hookName)
	ctx = ctrl.LoggerInto(ctx, log)

	gvh, err := c.catalog.GroupVersionHook(hook)
	if err != nil {
		return errors.Wrapf(err, "failed to call extension handlers for hook %q: failed to compute GroupVersionHook", hookName)
	}

	if err := c.catalog.ValidateRequest(gvh, request); err != nil {
		return errors.Wrapf(err, "failed to call extension handlers for hook %q: request object is invalid for hook", gvh.GroupHook())
	}

	if err := c.catalog.ValidateResponse(gvh, response); err != nil {
		return errors.Wrapf(err, "failed to call extension handlers for hook %q: response object is invalid for hook", gvh.GroupHook())
	}

	matchingHandlers, err := c.GetAllExtensions(ctx, hook, forObject)
	if err != nil {
		return errors.Wrapf(err, "failed to call extension handlers for hook %q", gvh.GroupHook())
	}

	responses := []runtimehooksv1.ResponseObject{}

	for _, handlerName := range matchingHandlers {
		responseObject, err := c.catalog.NewResponse(gvh)
		if err != nil {
			return errors.Wrapf(err, "failed to call extension handlers for hook %q: failed to call extension handler %q", gvh.GroupHook(), handlerName)
		}

		tmpResponse, ok := responseObject.(runtimehooksv1.ResponseObject)
		if !ok {
			return errors.Errorf(
				"failed to call extension handlers for hook %q: failed to call extension handler %q: response is not a ResponseObject",
				gvh.GroupHook(), handlerName)
		}

		err = c.CallExtension(ctx, hook, forObject, handlerName, request, tmpResponse)
		if err != nil {
			log.Error(err, "failed to call extension handlers")

			return errors.Wrapf(err, "failed to call extension handlers for hook %q", gvh.GroupHook())
		}

		responses = append(responses, tmpResponse)
	}

	aggregateSuccessfulResponses(response, responses)

	return nil
}

func aggregateSuccessfulResponses(aggregatedResponse runtimehooksv1.ResponseObject, responses []runtimehooksv1.ResponseObject) {
	aggregatedResponse.SetStatus(runtimehooksv1.ResponseStatusSuccess)

	messages := []string{}

	for _, resp := range responses {
		aggregatedRetryResponse, ok := aggregatedResponse.(runtimehooksv1.RetryResponseObject)
		if ok {
			if retryResp, retryOk := resp.(runtimehooksv1.RetryResponseObject); retryOk {
				aggregatedRetryResponse.SetRetryAfterSeconds(util.LowestNonZeroInt32(
					aggregatedRetryResponse.GetRetryAfterSeconds(),
					retryResp.GetRetryAfterSeconds(),
				))
			}
		}

		if resp.GetMessage() != "" {
			messages = append(messages, resp.GetMessage())
		}
	}

	aggregatedResponse.SetMessage(strings.Join(messages, ", "))
}

func (c *client) CallExtension(
	ctx context.Context,
	hook runtimecatalog.Hook,
	forObject ctrlclient.Object,
	name string,
	request runtimehooksv1.RequestObject,
	response runtimehooksv1.ResponseObject,
	opts ...runtimeclientapi.CallExtensionOption,
) error {
	options := &runtimeclientapi.CallExtensionOptions{}
	for _, opt := range opts {
		opt.ApplyToOptions(options)
	}

	log := ctrl.LoggerFrom(ctx).WithValues("extensionHandler", name, "hook", runtimecatalog.HookName(hook))
	ctx = ctrl.LoggerInto(ctx, log)

	hookGVH, err := c.catalog.GroupVersionHook(hook)
	if err != nil {
		return errors.Wrapf(err, "failed to call extension handler %q: failed to compute GroupVersionHook", name)
	}

	if err := c.catalog.ValidateRequest(hookGVH, request); err != nil {
		return errors.Wrapf(err, "failed to call extension handler %q: request object is invalid for hook %q", name, hookGVH)
	}

	if err := c.catalog.ValidateResponse(hookGVH, response); err != nil {
		return errors.Wrapf(err, "failed to call extension handler %q: response object is invalid for hook %q", name, hookGVH)
	}

	registration, err := c.registry.Get(name)
	if err != nil {
		return errors.Wrapf(err, "failed to call extension handler %q", name)
	}

	if hookGVH.GroupHook() != registration.GroupVersionHook.GroupHook() {
		return errors.Errorf("failed to call extension handler %q: handler does not match GroupHook %q", name, hookGVH.GroupHook())
	}

	namespaceMatches, err := c.matchNamespace(ctx, registration.NamespaceSelector, forObject.GetNamespace())
	if err != nil {
		return errors.Errorf("failed to call extension handler %q", name)
	}

	if !namespaceMatches {
		return errors.Errorf("failed to call extension handler %q: namespaceSelector did not match object %s", name, util.ObjectKey(forObject))
	}

	log.V(4).Info(fmt.Sprintf("Calling extension handler %q", name))

	timeoutDuration := runtimehooksv1.DefaultHandlersTimeoutSeconds * time.Second
	if registration.TimeoutSeconds != 0 {
		timeoutDuration = time.Duration(registration.TimeoutSeconds) * time.Second
	}

	request = cloneAndAddSettings(request, registration.Settings)

	var cacheKey string
	if options.WithCaching {
		cacheKey = options.CacheKeyFunc(registration.Name, registration.ExtensionConfigResourceVersion, request)
		if cacheEntry, ok := options.Cache.Has(cacheKey); ok {
			outVal := reflect.ValueOf(response)

			cacheVal := reflect.ValueOf(cacheEntry.Response)
			if !cacheVal.Type().AssignableTo(outVal.Type()) {
				return fmt.Errorf("failed to call extension handler %q: cached response of type %s instead of type %s", name, cacheVal.Type(), outVal.Type())
			}

			reflect.Indirect(outVal).Set(reflect.Indirect(cacheVal))

			return nil
		}
	}

	httpClient, err := c.getHTTPClient(registration.ClientConfig)
	if err != nil {
		return errors.Wrapf(err, "failed to call extension handler %q: failed to get http client", name)
	}

	httpOpts := &httpCallOptions{
		catalog:         c.catalog,
		config:          registration.ClientConfig,
		registrationGVH: registration.GroupVersionHook,
		hookGVH:         hookGVH,
		name:            strings.TrimSuffix(registration.Name, "."+registration.ExtensionConfigName),
		timeout:         timeoutDuration,
		httpClient:      httpClient,
	}

	err = httpCall(ctx, request, response, httpOpts)
	if err != nil {
		ignore := registration.FailurePolicy == runtimev1.FailurePolicyIgnore

		var errCallingExtensionHandler errCallingExtensionHandler
		if ignore && errors.As(err, &errCallingExtensionHandler) {
			log.Error(err, fmt.Sprintf("Ignoring error calling extension handler because of FailurePolicy %q", registration.FailurePolicy))
			response.SetStatus(runtimehooksv1.ResponseStatusSuccess)
			response.SetMessage("")

			return nil
		}

		log.Error(err, "Failed to call extension handler")

		return errors.Wrapf(err, "failed to call extension handler %q", name)
	}

	if err := validateResponseStatus(log, response, "call extension handler", name); err != nil {
		return err
	}

	if retryResponse, ok := response.(runtimehooksv1.RetryResponseObject); ok && retryResponse.GetRetryAfterSeconds() != 0 {
		log.V(4).Info(fmt.Sprintf("Extension handler returned blocking response with retryAfterSeconds of %d", retryResponse.GetRetryAfterSeconds()))
	} else {
		log.V(4).Info("Extension handler returned success response")
	}

	if options.WithCaching {
		options.Cache.Add(runtimeclientapi.CallExtensionCacheEntry{
			CacheKey: cacheKey,
			Response: response,
		})
	}

	return nil
}

func (c *client) getHTTPClient(config runtimev1.ClientConfig) (*http.Client, error) {
	extensionURL, err := urlForExtension(config, runtimecatalog.GroupVersionHook{}, "")
	if err != nil {
		return nil, err
	}

	if cacheEntry, ok := c.httpClientsCache.Has(newHTTPClientEntryKey(extensionURL.Hostname(), config.CABundle)); ok {
		return cacheEntry.client, nil
	}

	httpClient, err := createHTTPClient(c.certFile, c.keyFile, config.CABundle, extensionURL.Hostname())
	if err != nil {
		return nil, err
	}

	c.httpClientsCache.Add(newHTTPClientEntry(extensionURL.Hostname(), config.CABundle, httpClient))

	return httpClient, nil
}

func createHTTPClient(certFile, keyFile string, caData []byte, hostName string) (*http.Client, error) {
	httpClient := &http.Client{}

	tlsConfig, err := transport.TLSConfigFor(&transport.Config{
		TLS: transport.TLSConfig{
			CertFile:   certFile,
			KeyFile:    keyFile,
			CAData:     caData,
			ServerName: hostName,
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create tls config")
	}

	httpClient.Transport = utilnet.SetTransportDefaults(&http.Transport{
		TLSClientConfig: tlsConfig,
	})

	return httpClient, nil
}

func cloneAndAddSettings(request runtimehooksv1.RequestObject, registrationSettings map[string]string) runtimehooksv1.RequestObject {
	requestCopy, ok := request.DeepCopyObject().(runtimehooksv1.RequestObject)
	if !ok {
		return request
	}

	request = requestCopy

	settings := map[string]string{}
	maps.Copy(settings, registrationSettings)

	maps.Copy(settings, request.GetSettings())

	request.SetSettings(settings)

	return request
}

type httpCallOptions struct {
	catalog         *runtimecatalog.Catalog
	config          runtimev1.ClientConfig
	registrationGVH runtimecatalog.GroupVersionHook
	hookGVH         runtimecatalog.GroupVersionHook
	name            string
	timeout         time.Duration
	httpClient      *http.Client
}

func httpCall(ctx context.Context, request, response runtime.Object, opts *httpCallOptions) error {
	log := ctrl.LoggerFrom(ctx)

	if opts == nil || request == nil || response == nil {
		return errors.New("http call failed: opts, request and response cannot be nil")
	}

	if opts.catalog == nil {
		return errors.New("http call failed: opts.Catalog cannot be nil")
	}

	extensionURL, err := urlForExtension(opts.config, opts.registrationGVH, opts.name)
	if err != nil {
		return errors.Wrap(err, "http call failed")
	}

	start := time.Now()

	defer func() {
		defaultMetrics.RequestDuration.Observe(opts.hookGVH, *extensionURL, time.Since(start))
	}()

	requireConversion := opts.registrationGVH.Version != opts.hookGVH.Version

	requestLocal := request
	responseLocal := response

	if requireConversion {
		log.V(5).Info(fmt.Sprintf("Hook version of supported request is %s. Converting request from %s", opts.registrationGVH, opts.hookGVH))

		var err error

		requestLocal, err = opts.catalog.NewRequest(opts.registrationGVH)
		if err != nil {
			return errors.Wrap(err, "http call failed")
		}

		if err := opts.catalog.Convert(request, requestLocal, ctx); err != nil {
			return errors.Wrapf(err, "http call failed: failed to convert request from %T to %T", request, requestLocal)
		}

		responseLocal, err = opts.catalog.NewResponse(opts.registrationGVH)
		if err != nil {
			return errors.Wrap(err, "http call failed")
		}
	}

	requestGVH, err := opts.catalog.Request(opts.registrationGVH)
	if err != nil {
		return errors.Wrap(err, "http call failed")
	}

	requestLocal.GetObjectKind().SetGroupVersionKind(requestGVH)

	postBody, err := json.Marshal(requestLocal)
	if err != nil {
		return errors.Wrap(err, "http call failed: failed to marshall request object")
	}

	if opts.timeout != 0 {
		values := extensionURL.Query()
		values.Add("timeout", opts.timeout.String())
		extensionURL.RawQuery = values.Encode()

		var cancel context.CancelFunc

		ctx, cancel = context.WithTimeoutCause(ctx, opts.timeout, errors.New("http request timeout expired"))
		defer cancel()
	}

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, extensionURL.String(), bytes.NewBuffer(postBody))
	if err != nil {
		return errors.Wrap(err, "http call failed: failed to create http request")
	}

	resp, err := opts.httpClient.Do(httpRequest)

	defer func() {
		defaultMetrics.RequestsTotal.Observe(httpRequest, resp, opts.hookGVH, err, response)
	}()

	if err != nil {
		return errCallingExtensionHandler(
			errors.Wrapf(err, "http call failed"),
		)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return errCallingExtensionHandler(
				errors.Errorf("http call failed: got response with status code %d != 200: failed to read response body", resp.StatusCode),
			)
		}

		return errCallingExtensionHandler(
			errors.Errorf("http call failed: got response with status code %d != 200: response: %q", resp.StatusCode, string(respBody)),
		)
	}

	if err := json.NewDecoder(resp.Body).Decode(responseLocal); err != nil {
		return errCallingExtensionHandler(
			errors.Wrap(err, "http call failed: failed to decode response"),
		)
	}

	if requireConversion {
		log.V(5).Info(fmt.Sprintf("Hook version of received response is %s. Converting response to %s", opts.registrationGVH, opts.hookGVH))

		if err := opts.catalog.Convert(responseLocal, response, ctx); err != nil {
			return errors.Wrapf(err, "http call failed: failed to convert response from %T to %T", requestLocal, response)
		}
	}

	return nil
}

func urlForExtension(config runtimev1.ClientConfig, gvh runtimecatalog.GroupVersionHook, name string) (*url.URL, error) {
	var u *url.URL

	if config.Service.IsDefined() {
		svc := config.Service

		host := svc.Name + "." + svc.Namespace + ".svc"
		if svc.Port != nil {
			host = net.JoinHostPort(host, strconv.Itoa(int(*svc.Port)))
		}

		u = &url.URL{
			Scheme: "https",
			Host:   host,
		}
		if svc.Path != "" {
			u.Path = svc.Path
		}
	} else {
		if config.URL == "" {
			return nil, errors.New("failed to compute URL: at least one of service and url should be defined in config")
		}

		var err error

		u, err = url.Parse(config.URL)
		if err != nil {
			return nil, errors.Wrap(err, "failed to compute URL: failed to parse url from clientConfig")
		}

		if u.Scheme != "https" {
			return nil, errors.Errorf("failed to compute URL: expected https scheme, got %s", u.Scheme)
		}
	}

	u.Path = path.Join(u.Path, runtimecatalog.GVHToPath(gvh, name))

	return u, nil
}

func defaultAndValidateDiscoveryResponse(cat *runtimecatalog.Catalog, discovery *runtimehooksv1.DiscoveryResponse) error {
	if discovery == nil {
		return errors.New("failed to validate discovery response: response is nil")
	}

	discovery = defaultDiscoveryResponse(discovery)

	var errs []error

	names := make(map[string]bool)
	for _, handler := range discovery.Handlers {
		if _, ok := names[handler.Name]; ok {
			errs = append(errs, errors.Errorf("duplicate name for handler %s found", handler.Name))
		}

		names[handler.Name] = true

		if errStrings := validation.IsDNS1123Label(handler.Name); len(errStrings) > 0 {
			errs = append(errs, errors.Errorf("handler name %s is not valid: %s", handler.Name, errStrings))
		}

		if *handler.TimeoutSeconds < 0 || *handler.TimeoutSeconds > 30 {
			errs = append(errs, errors.Errorf("handler %s timeoutSeconds %d must be between 0 and 30", handler.Name, *handler.TimeoutSeconds))
		}

		if *handler.FailurePolicy != runtimehooksv1.FailurePolicyFail && *handler.FailurePolicy != runtimehooksv1.FailurePolicyIgnore {
			errs = append(errs, errors.Errorf("handler %s failurePolicy %s must equal \"Ignore\" or \"Fail\"", handler.Name, *handler.FailurePolicy))
		}

		gv, err := schema.ParseGroupVersion(handler.RequestHook.APIVersion)
		if err != nil {
			errs = append(errs, errors.Wrapf(err, "handler %s requestHook APIVersion %s is not valid", handler.Name, handler.RequestHook.APIVersion))
		} else if !cat.IsHookRegistered(runtimecatalog.GroupVersionHook{
			Group:   gv.Group,
			Version: gv.Version,
			Hook:    handler.RequestHook.Hook,
		}) {
			errs = append(errs, errors.Errorf(
				"handler %s requestHook %s/%s is not in the Runtime SDK catalog",
				handler.Name, handler.RequestHook.APIVersion, handler.RequestHook.Hook))
		}
	}

	return errors.Wrapf(kerrors.NewAggregate(errs), "failed to validate discovery response")
}

func defaultDiscoveryResponse(discovery *runtimehooksv1.DiscoveryResponse) *runtimehooksv1.DiscoveryResponse {
	for i, handler := range discovery.Handlers {
		if handler.FailurePolicy == nil {
			defaultFailPolicy := runtimehooksv1.FailurePolicyFail
			handler.FailurePolicy = &defaultFailPolicy
		}

		if handler.TimeoutSeconds == nil {
			handler.TimeoutSeconds = ptr.To[int32](runtimehooksv1.DefaultHandlersTimeoutSeconds)
		}

		discovery.Handlers[i] = handler
	}

	return discovery
}

func (c *client) matchNamespace(ctx context.Context, selector labels.Selector, namespace string) (bool, error) {
	if selector.Empty() {
		return true, nil
	}

	ns := &metav1.PartialObjectMetadata{}
	ns.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Namespace",
	})

	if err := c.client.Get(ctx, ctrlclient.ObjectKey{Name: namespace}, ns); err != nil {
		return false, errors.Wrapf(err, "failed to match namespace: failed to get namespace %s", namespace)
	}

	return selector.Matches(labels.Set(ns.GetLabels())), nil
}

func nameForHandler(handler runtimehooksv1.ExtensionHandler, extensionConfig *runtimev1.ExtensionConfig) (string, error) {
	if extensionConfig == nil {
		return "", errors.New("extensionConfig was nil")
	}

	return handler.Name + "." + extensionConfig.Name, nil
}

func validateResponseStatus(log logr.Logger, response runtimehooksv1.ResponseObject, operationName, targetName string) error {
	if response.GetStatus() != runtimehooksv1.ResponseStatusSuccess {
		if response.GetStatus() == runtimehooksv1.ResponseStatusFailure {
			log.Info(fmt.Sprintf("Failed to %s %q: got failure response with message %v", operationName, targetName, response.GetMessage()))

			return errors.Errorf("failed to %s %q: got failure response, please check controller logs for errors", operationName, targetName)
		}

		log.Info(fmt.Sprintf(
			"Failed to %s %q: got unknown response status %q with message %v",
			operationName, targetName, response.GetStatus(), response.GetMessage()))

		return errors.Errorf(
			"failed to %s %q: got unknown response status %q, please check controller logs for errors",
			operationName, targetName, response.GetStatus())
	}

	return nil
}
