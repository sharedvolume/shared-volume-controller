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

package processors

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/sharedvolume/shared-volume-controller/internal/webhook/types"
	"github.com/sharedvolume/shared-volume-controller/internal/webhook/utils"
)

// AnnotationProcessor handles annotation extraction and processing
type AnnotationProcessor struct {
	config *types.WebhookConfig
}

// NewAnnotationProcessor creates a new annotation processor
func NewAnnotationProcessor(config *types.WebhookConfig) *AnnotationProcessor {
	return &AnnotationProcessor{
		config: config,
	}
}

// Process implements the WebhookProcessor interface
func (ap *AnnotationProcessor) Process(ctx context.Context, pod *corev1.Pod, req admission.Request) error {
	// Extract volume annotations from the pod
	volumes := ap.ExtractVolumeAnnotations(pod)

	// Add any missing annotations back to the pod
	if len(volumes) > 0 {
		return ap.AddAnnotations(pod, volumes)
	}

	return nil
}

// ExtractVolumeAnnotations extracts volume references from pod annotations
func (ap *AnnotationProcessor) ExtractVolumeAnnotations(pod *corev1.Pod) []types.VolumeRef {
	return utils.ExtractVolumeAnnotations(pod)
}

// ExtractVolumeReferences extracts volume references from pod annotations (alias for compatibility)
func (ap *AnnotationProcessor) ExtractVolumeReferences(pod *corev1.Pod) []types.VolumeRef {
	return ap.ExtractVolumeAnnotations(pod)
}

// AddAnnotations adds volume annotations to the pod
func (ap *AnnotationProcessor) AddAnnotations(pod *corev1.Pod, volumes []types.VolumeRef) error {
	utils.AddVolumeAnnotations(pod, volumes)
	return nil
}

// HasVolumeAnnotations checks if the pod has volume annotations
func (ap *AnnotationProcessor) HasVolumeAnnotations(pod *corev1.Pod) bool {
	return utils.HasVolumeAnnotations(pod)
}
