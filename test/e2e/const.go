//go:build e2e
// +build e2e

/*
Copyright 2024 SUSE.

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

package e2e

import (
	_ "embed"
)

const (
	// these timeouts are used for setting control plane
	// - `NodeDrainTimeoutSeconds`
	// - `NodeDeletionTimeoutSeconds
	// - `NodeVolumeDetachTimeoutSeconds`
	timeout240s = 240
	timeout480s = 480
)

var (
	//go:embed data/infrastructure/clusterclass-template-docker.yaml
	ClusterClassDocker []byte
	//go:embed data/infrastructure/cluster-from-clusterclass-template-docker.yaml
	ClusterFromClusterClassDocker []byte
	//go:embed data/infrastructure/cluster-template-docker-external-datastore.yaml
	ClusterTemplateDockerExternalDatastore []byte
	//go:embed data/infrastructure/postgres.yaml
	Postgres []byte
)
