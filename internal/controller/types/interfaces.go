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

	"sigs.k8s.io/controller-runtime/pkg/client"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

// VolumeObject represents a common interface for volume-like objects
type VolumeObject interface {
	client.Object
	// GetVolumeSpec returns the common volume specification
	GetVolumeSpec() *svv1alpha1.VolumeSpecBase
	// GetPhase returns the current phase
	GetPhase() string
	// SetPhase sets the current phase
	SetPhase(phase string)
	// GetMessage returns the current message
	GetMessage() string
	// SetMessage sets the current message
	SetMessage(message string)
	// GetNfsServerAddress returns the NFS server address
	GetNfsServerAddress() string
	// SetNfsServerAddress sets the NFS server address
	SetNfsServerAddress(address string)
	// GetPersistentVolumeClaimName returns the PVC name
	GetPersistentVolumeClaimName() string
	// SetPersistentVolumeClaimName sets the PVC name
	SetPersistentVolumeClaimName(name string)
	// GetPersistentVolumeName returns the PV name
	GetPersistentVolumeName() string
	// SetPersistentVolumeName sets the PV name
	SetPersistentVolumeName(name string)
	// GetServiceName returns the service name
	GetServiceName() string
	// SetServiceName sets the service name
	SetServiceName(name string)
}

// ResourceManager interface for managing Kubernetes resources
type ResourceManager interface {
	// Reconcile ensures the resource exists and is in the desired state
	Reconcile(ctx context.Context, volumeObj VolumeObject, namespace string) error
	// IsReady checks if the resource is ready
	IsReady(ctx context.Context, volumeObj VolumeObject, namespace string) (bool, error)
}

// StatusManager interface for managing volume status
type StatusManager interface {
	// UpdateStatus updates the volume status
	UpdateStatus(ctx context.Context, volumeObj VolumeObject, nfsReady bool, phase string, message string) error
	// UpdateSimpleStatus updates the volume status with just phase and message
	UpdateSimpleStatus(ctx context.Context, volumeObj VolumeObject, phase string, message string) error
}

// VolumeRef represents a reference to either a SharedVolume or ClusterSharedVolume
type VolumeRef struct {
	Name      string
	Namespace string
	IsCluster bool // true for ClusterSharedVolume, false for SharedVolume
}

// SharedVolumeRef represents a reference to a SharedVolume (legacy, for backward compatibility)
type SharedVolumeRef struct {
	Namespace string
	Name      string
}
