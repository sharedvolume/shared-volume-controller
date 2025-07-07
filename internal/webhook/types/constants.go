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
	// PVPVCNameTemplate is the template for PV and PVC names
	PVPVCNameTemplate = "%s-%s"

	// SharedVolumePodFinalizer is the finalizer added to pods using SharedVolumes
	SharedVolumePodFinalizer = "sharedvolume.sv/pod-cleanup"

	// SharedVolumeAnnotationKey for SharedVolume annotation
	SharedVolumeAnnotationKey = "sharedvolume.sv"

	// ClusterSharedVolumeAnnotationKey for ClusterSharedVolume annotation
	ClusterSharedVolumeAnnotationKey = "sharedvolume.csv"

	// ClusterSharedVolumeOperationNamespace is the namespace used for ClusterSharedVolume operations
	ClusterSharedVolumeOperationNamespace = "shared-volume-controller-operation"

	// DefaultMountPath is the default mount path for shared volumes
	DefaultMountPath = "/shared"

	// DefaultStorageSize is the default storage size for PVCs
	DefaultStorageSize = "10Gi"

	// NFSCSIDriverName is the name of the NFS CSI driver
	NFSCSIDriverName = "nfs.csi.k8s.io"
)
