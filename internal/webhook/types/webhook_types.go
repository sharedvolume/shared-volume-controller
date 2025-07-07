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

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// VolumeRef represents a reference to either a SharedVolume or ClusterSharedVolume
type VolumeRef struct {
	Name      string
	Namespace string
	IsCluster bool // true for ClusterSharedVolume, false for SharedVolume
}

// VolumeReference is an alias for VolumeRef for compatibility
type VolumeReference = VolumeRef

// WebhookProcessor defines the interface for webhook processing components
type WebhookProcessor interface {
	Process(ctx context.Context, pod *corev1.Pod, req admission.Request) error
}

// AnnotationProcessor handles annotation extraction and processing
type AnnotationProcessor interface {
	ExtractVolumeAnnotations(pod *corev1.Pod) []VolumeRef
	AddAnnotations(pod *corev1.Pod, volumes []VolumeRef) error
}

// VolumeProcessor handles volume and PVC creation
type VolumeProcessor interface {
	ProcessVolumes(ctx context.Context, pod *corev1.Pod, volumes []VolumeRef) error
	CreatePVCsForVolumes(ctx context.Context, pod *corev1.Pod, volumes []VolumeRef) error
}

// FinalizerProcessor handles finalizer management
type FinalizerProcessor interface {
	AddFinalizer(pod *corev1.Pod, finalizer string) bool
	RemoveFinalizer(pod *corev1.Pod, finalizer string) bool
	HasFinalizer(pod *corev1.Pod, finalizer string) bool
}

// Validator handles validation logic
type Validator interface {
	ValidatePod(ctx context.Context, pod *corev1.Pod) error
	ValidateVolumes(ctx context.Context, volumes []VolumeRef) error
}

// Mutator handles pod mutations
type Mutator interface {
	MutatePod(ctx context.Context, pod *corev1.Pod, volumes []VolumeRef) error
}

// WebhookConfig holds configuration for the webhook
type WebhookConfig struct {
	Client              client.Client
	SkipValidation      bool
	DefaultStorageClass string
	DefaultMountPath    string
}
