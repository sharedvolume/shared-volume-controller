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

// ClusterSharedVolumeSpec defines the desired state of ClusterSharedVolume
type ClusterSharedVolumeSpec struct {
	// VolumeSpecBase contains the common volume specification
	VolumeSpecBase `json:",inline"`
}

// ClusterSharedVolumeStatus defines the observed state of ClusterSharedVolume
type ClusterSharedVolumeStatus struct {
	// Phase represents the current phase of the ClusterSharedVolume
	Phase string `json:"phase,omitempty"`

	// Message provides additional information about the current state
	Message string `json:"message,omitempty"`

	// NfsServerAddress is the address of the NFS server
	NfsServerAddress string `json:"nfsServerAddress,omitempty"`

	// PersistentVolumeClaimName is the name of the created PVC
	PersistentVolumeClaimName string `json:"persistentVolumeClaimName,omitempty"`

	// PersistentVolumeName is the name of the created PV
	PersistentVolumeName string `json:"persistentVolumeName,omitempty"`

	// ServiceName is the name of the service exposing the NFS server
	ServiceName string `json:"serviceName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="NFS Address",type=string,JSONPath=`.status.nfsServerAddress`
// +kubebuilder:printcolumn:name="Mount Path",type=string,JSONPath=`.spec.mountPath`
// +kubebuilder:printcolumn:name="SyncInterval",type=string,JSONPath=`.spec.syncInterval`
// +kubebuilder:printcolumn:name="Capacity",type=string,JSONPath=`.spec.storage.capacity`

// ClusterSharedVolume is the Schema for the clustersharedvolumes API
type ClusterSharedVolume struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSharedVolumeSpec   `json:"spec,omitempty"`
	Status ClusterSharedVolumeStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ClusterSharedVolumeList contains a list of ClusterSharedVolume
type ClusterSharedVolumeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterSharedVolume `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterSharedVolume{}, &ClusterSharedVolumeList{})
}

// VolumeObject interface implementation for ClusterSharedVolume

// GetVolumeSpec returns the common volume specification
func (csv *ClusterSharedVolume) GetVolumeSpec() *VolumeSpecBase {
	return &csv.Spec.VolumeSpecBase
}

// GetPhase returns the current phase
func (csv *ClusterSharedVolume) GetPhase() string {
	return csv.Status.Phase
}

// SetPhase sets the current phase
func (csv *ClusterSharedVolume) SetPhase(phase string) {
	csv.Status.Phase = phase
}

// GetMessage returns the current message
func (csv *ClusterSharedVolume) GetMessage() string {
	return csv.Status.Message
}

// SetMessage sets the current message
func (csv *ClusterSharedVolume) SetMessage(message string) {
	csv.Status.Message = message
}

// GetNfsServerAddress returns the NFS server address
func (csv *ClusterSharedVolume) GetNfsServerAddress() string {
	return csv.Status.NfsServerAddress
}

// SetNfsServerAddress sets the NFS server address
func (csv *ClusterSharedVolume) SetNfsServerAddress(address string) {
	csv.Status.NfsServerAddress = address
}

// GetPersistentVolumeClaimName returns the PVC name
func (csv *ClusterSharedVolume) GetPersistentVolumeClaimName() string {
	return csv.Status.PersistentVolumeClaimName
}

// SetPersistentVolumeClaimName sets the PVC name
func (csv *ClusterSharedVolume) SetPersistentVolumeClaimName(name string) {
	csv.Status.PersistentVolumeClaimName = name
}

// GetPersistentVolumeName returns the PV name
func (csv *ClusterSharedVolume) GetPersistentVolumeName() string {
	return csv.Status.PersistentVolumeName
}

// SetPersistentVolumeName sets the PV name
func (csv *ClusterSharedVolume) SetPersistentVolumeName(name string) {
	csv.Status.PersistentVolumeName = name
}

// GetServiceName returns the service name
func (csv *ClusterSharedVolume) GetServiceName() string {
	return csv.Status.ServiceName
}

// SetServiceName sets the service name
func (csv *ClusterSharedVolume) SetServiceName(name string) {
	csv.Status.ServiceName = name
}
