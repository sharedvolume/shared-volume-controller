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

package validators

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/sharedvolume/shared-volume-controller/internal/webhook/types"
)

// PodValidator handles pod validation logic
type PodValidator struct{}

// NewPodValidator creates a new pod validator
func NewPodValidator() *PodValidator {
	return &PodValidator{}
}

// ValidateForVolumeProcessing validates that a pod is ready for volume processing
func (v *PodValidator) ValidateForVolumeProcessing(pod *corev1.Pod, volumes []types.VolumeRef) error {
	if pod == nil {
		return fmt.Errorf("pod cannot be nil")
	}

	if len(volumes) == 0 {
		return fmt.Errorf("no volumes provided for processing")
	}

	// Validate pod has containers
	if len(pod.Spec.Containers) == 0 {
		return fmt.Errorf("pod must have at least one container")
	}

	// Validate volume references
	for _, vol := range volumes {
		if vol.Name == "" {
			return fmt.Errorf("volume name cannot be empty")
		}

		// For regular SharedVolume, namespace should match pod namespace
		if !vol.IsCluster && vol.Namespace != pod.Namespace {
			return fmt.Errorf("SharedVolume namespace %s does not match pod namespace %s", vol.Namespace, pod.Namespace)
		}
	}

	return nil
}

// ValidateVolumeName validates that a volume name is valid
func (v *PodValidator) ValidateVolumeName(name string) error {
	if name == "" {
		return fmt.Errorf("volume name cannot be empty")
	}

	// Add more validation as needed
	return nil
}

// ValidatePodForDeletion validates that a pod is ready for deletion cleanup
func (v *PodValidator) ValidatePodForDeletion(pod *corev1.Pod) error {
	if pod == nil {
		return fmt.Errorf("pod cannot be nil")
	}

	if pod.DeletionTimestamp == nil {
		return fmt.Errorf("pod is not being deleted")
	}

	return nil
}
