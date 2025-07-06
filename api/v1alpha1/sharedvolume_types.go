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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// SharedVolumeSpec defines the desired state of SharedVolume.
type SharedVolumeSpec struct {
	VolumeSpecBase `json:",inline"`
}

// SharedVolumeStatus defines the observed state of SharedVolume.
type SharedVolumeStatus struct {
	// Phase indicates the current phase of the SharedVolume (Pending, Preparing, Ready)
	Phase string `json:"phase,omitempty"`
	// Message provides additional information about the current phase
	Message string `json:"message,omitempty"`
	// NfsServerAddress is the address where the NFS server can be accessed
	NfsServerAddress string `json:"nfsServerAddress,omitempty"`
	// PersistentVolumeClaimName is the name of the associated PVC
	PersistentVolumeClaimName string `json:"persistentVolumeClaimName,omitempty"`
	// PersistentVolumeName is the name of the associated PV
	PersistentVolumeName string `json:"persistentVolumeName,omitempty"`
	// ServiceName is the name of the associated Service
	ServiceName string `json:"serviceName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=sv,scope=Namespaced
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=".status.phase",description="Current Phase"
// +kubebuilder:printcolumn:name="NFS Address",type=string,JSONPath=".status.nfsServerAddress",description="NFS Server Address"
// +kubebuilder:printcolumn:name="Mount Path",type=string,JSONPath=".spec.mountPath",description="Mount Path"
// +kubebuilder:printcolumn:name="SyncInterval",type=string,JSONPath=".spec.syncInterval",description="Sync Interval"
// +kubebuilder:printcolumn:name="Capacity",type=string,JSONPath=".spec.storage.capacity",description="Storage Capacity"

// SharedVolume is the Schema for the sharedvolumes API.
type SharedVolume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SharedVolumeSpec   `json:"spec,omitempty"`
	Status SharedVolumeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// SharedVolumeList contains a list of SharedVolume.
type SharedVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SharedVolume `json:"items"`
}

func init() {
	SchemeBuilder.Register(&SharedVolume{}, &SharedVolumeList{})
}
