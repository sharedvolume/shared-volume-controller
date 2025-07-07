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

const (
	// Volume Controller Constants

	// NfsServerGeneratingAnnotation marks when NfsServer generation is in progress
	NfsServerGeneratingAnnotation = "shared-volume.io/nfsserver-generating"

	// ClusterSharedVolume Constants

	// ClusterSharedVolumeOperationNamespace is the namespace used for ClusterSharedVolume operations
	ClusterSharedVolumeOperationNamespace = "shared-volume-controller-operation"

	// Pod Cleanup Constants

	// SharedVolumePodFinalizer is the finalizer added to pods using SharedVolumes
	SharedVolumePodFinalizer = "sharedvolume.sv/pod-cleanup"

	// Annotation Keys

	// SharedVolumeAnnotationKey is the annotation key for SharedVolume references
	SharedVolumeAnnotationKey = "sharedvolume.sv"

	// ClusterSharedVolumeAnnotationKey is the annotation key for ClusterSharedVolume references
	ClusterSharedVolumeAnnotationKey = "sharedvolume.csv"

	// Phase Constants

	// VolumePhaseInitializing indicates the volume is being initialized
	VolumePhaseInitializing = "Initializing"

	// VolumePhaseCreatingNfsServer indicates NFS server is being created
	VolumePhaseCreatingNfsServer = "CreatingNfsServer"

	// VolumePhaseWaitingForNfsServer indicates waiting for NFS server to be ready
	VolumePhaseWaitingForNfsServer = "WaitingForNfsServer"

	// VolumePhaseCreatingResources indicates resources are being created
	VolumePhaseCreatingResources = "CreatingResources"

	// VolumePhaseReady indicates the volume is ready for use
	VolumePhaseReady = "Ready"

	// VolumePhaseError indicates an error occurred
	VolumePhaseError = "Error"

	// Resource Name Templates

	// PVCNameTemplate is the template for PVC names
	PVCNameTemplate = "%s-%s" // referenceValue-namespace

	// PVNameTemplate is the template for PV names
	PVNameTemplate = "%s-%s" // referenceValue-namespace

	// ServiceNameTemplate is the template for service names
	ServiceNameTemplate = "%s-service" // referenceValue-service

	// ReplicaSetNameTemplate is the template for ReplicaSet names
	ReplicaSetNameTemplate = "%s-replicaset" // referenceValue-replicaset

	// Container and Image Constants

	// DefaultNfsServerImage is the default NFS server container image
	DefaultNfsServerImage = "itsthenetwork/nfs-server-alpine:latest"

	// DefaultNfsServerPort is the default NFS server port
	DefaultNfsServerPort = 2049

	// DefaultRpcBindPort is the default RPC bind port
	DefaultRpcBindPort = 111

	// Retry Constants

	// DefaultMaxRetries is the default maximum number of retries
	DefaultMaxRetries = 5

	// DefaultRetryDelay is the default delay between retries
	DefaultRetryDelay = "1s"

	// Storage Constants

	// DefaultStorageCapacity is the default storage capacity
	DefaultStorageCapacity = "1Gi"

	// DefaultAccessMode is the default access mode
	DefaultAccessMode = "ReadWriteMany"

	// Finalizer Constants

	// SharedVolumeFinalizer is the finalizer for SharedVolume resources
	SharedVolumeFinalizer = "sharedvolume.sv/finalizer"

	// ClusterSharedVolumeFinalizer is the finalizer for ClusterSharedVolume resources
	ClusterSharedVolumeFinalizer = "clustersharedvolume.sv/finalizer"
)
