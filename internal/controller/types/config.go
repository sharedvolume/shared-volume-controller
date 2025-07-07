/*
Copyright 2025.

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

package types

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// VolumeReconcileConfig holds the configuration for generic volume reconciliation
type VolumeReconcileConfig struct {
	VolumeObjFactory       func() VolumeObject
	APIVersion             string
	Kind                   string
	FinalizerName          string
	Namespace              string // The namespace to use for resource creation
	CreateReadyCallback    func() func(context.Context, VolumeObject) error
	CreateNotReadyCallback func() func(context.Context, VolumeObject) error
}

// ControllerSetupConfig holds configuration for generic controller setup
type ControllerSetupConfig struct {
	VolumeType          client.Object
	ControllerName      string
	ControllerNamespace string
	PreSetupHook        func(ctrl.Manager, interface{}) error // Changed to interface{} for flexibility
	Runnables           []manager.Runnable                    // Additional runnables to start with the manager
}

// ResourceConfig holds configuration for resource creation and management
type ResourceConfig struct {
	// Resource naming configuration
	ResourcePrefix string
	Namespace      string

	// NFS Server configuration
	GenerateNfsServer bool
	NfsServerImage    string
	NfsServerPort     int32
	RpcBindPort       int32

	// Storage configuration
	StorageCapacity string
	AccessModes     []string

	// Retry configuration
	MaxRetries int
	RetryDelay string
}

// ReconcileOptions holds options for reconciliation
type ReconcileOptions struct {
	// Whether to force reconciliation even if resources appear ready
	ForceReconcile bool

	// Whether to skip certain resource types during reconciliation
	SkipNfsServer  bool
	SkipPV         bool
	SkipPVC        bool
	SkipReplicaSet bool
	SkipService    bool

	// Custom resource configurations
	ResourceConfig *ResourceConfig
}
