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

package extensionconfig

import (
	"context"
	"time"

	"github.com/pkg/errors"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	runtimev1 "sigs.k8s.io/cluster-api/api/runtime/v1beta2"
	runtimeclient "sigs.k8s.io/cluster-api/exp/runtime/client"
)

const (
	defaultWarmupTimeout  = 60 * time.Second
	defaultWarmupInterval = 2 * time.Second
)

var _ manager.LeaderElectionRunnable = &warmupRunnable{}

type warmupRunnable struct {
	Client         client.Client
	APIReader      client.Reader
	RuntimeClient  runtimeclient.Client
	ReadOnly       bool
	warmupTimeout  time.Duration
	warmupInterval time.Duration
}

func (r *warmupRunnable) NeedLeaderElection() bool {
	return true
}

func (r *warmupRunnable) Start(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	if r.warmupInterval == 0 {
		r.warmupInterval = defaultWarmupInterval
	}

	if r.warmupTimeout == 0 {
		r.warmupTimeout = defaultWarmupTimeout
	}

	ctx, cancel := context.WithTimeoutCause(ctx, r.warmupTimeout, errors.New("warmup timeout expired"))
	defer cancel()

	var warmupErr error

	err := wait.PollUntilContextTimeout(ctx, r.warmupInterval, r.warmupTimeout, true, func(ctx context.Context) (done bool, err error) {
		if warmupErr = r.warmupRegistry(ctx); warmupErr != nil {
			log.Error(warmupErr, "ExtensionConfig registry warmup failed")

			return false, nil //nolint:nilerr
		}

		return true, nil
	})
	if err != nil {
		return errors.Wrapf(warmupErr, "ExtensionConfig registry warmup timed out after %s", r.warmupTimeout.String())
	}

	return nil
}

func (r *warmupRunnable) warmupRegistry(ctx context.Context) error {
	log := ctrl.LoggerFrom(ctx)

	extensionConfigList := runtimev1.ExtensionConfigList{}
	if err := r.APIReader.List(ctx, &extensionConfigList); err != nil {
		return errors.Wrapf(err, "failed to list ExtensionConfigs")
	}

	var errs []error

	for i := range extensionConfigList.Items {
		extensionConfig := &extensionConfigList.Items[i]

		log := log.WithValues("ExtensionConfig", klog.KObj(extensionConfig))
		ctx := ctrl.LoggerInto(ctx, log)

		if r.ReadOnly {
			if err := validateExtensionConfig(extensionConfig); err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to validate ExtensionConfig"))
			}
		} else {
			original := extensionConfig.DeepCopy()

			extensionConfig, err := reconcileExtensionConfig(ctx, r.Client, r.RuntimeClient, original, extensionConfig)
			if err != nil {
				errs = append(errs, errors.Wrapf(err, "failed to reconcile ExtensionConfig"))

				continue
			}

			extensionConfigList.Items[i] = *extensionConfig
		}
	}

	if len(errs) > 0 {
		return kerrors.NewAggregate(errs)
	}

	if err := r.RuntimeClient.WarmUp(&extensionConfigList); err != nil {
		return err
	}

	log.Info("The extension registry is warmed up")

	return nil
}
