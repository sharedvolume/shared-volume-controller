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
)

// CleanupValidator handles validation logic for cleanup operations
type CleanupValidator struct{}

// NewCleanupValidator creates a new cleanup validator
func NewCleanupValidator() *CleanupValidator {
	return &CleanupValidator{}
}

// ValidateForCleanup validates that a pod is ready for cleanup
func (v *CleanupValidator) ValidateForCleanup(pod *corev1.Pod) error {
	if pod == nil {
		return fmt.Errorf("pod cannot be nil")
	}

	if pod.DeletionTimestamp == nil {
		return fmt.Errorf("pod is not being deleted")
	}

	return nil
}

// ValidatePodForFinalizer validates that a pod can have a finalizer added/removed
func (v *CleanupValidator) ValidatePodForFinalizer(pod *corev1.Pod) error {
	if pod == nil {
		return fmt.Errorf("pod cannot be nil")
	}

	return nil
}
