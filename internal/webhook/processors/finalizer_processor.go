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
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/sharedvolume/shared-volume-controller/internal/webhook/types"
)

// FinalizerProcessor handles finalizer management for pods
type FinalizerProcessor struct {
	config *types.WebhookConfig
}

// NewFinalizerProcessor creates a new finalizer processor
func NewFinalizerProcessor(config *types.WebhookConfig) *FinalizerProcessor {
	return &FinalizerProcessor{
		config: config,
	}
}

// AddFinalizer adds a finalizer to the pod if it doesn't exist
func (fp *FinalizerProcessor) AddFinalizer(pod *corev1.Pod, finalizer string) bool {
	if fp.HasFinalizer(pod, finalizer) {
		return false // Finalizer already exists
	}

	pod.Finalizers = append(pod.Finalizers, finalizer)
	return true
}

// RemoveFinalizer removes a finalizer from the pod if it exists
func (fp *FinalizerProcessor) RemoveFinalizer(pod *corev1.Pod, finalizer string) bool {
	var newFinalizers []string
	found := false

	for _, f := range pod.Finalizers {
		if f != finalizer {
			newFinalizers = append(newFinalizers, f)
		} else {
			found = true
		}
	}

	if found {
		pod.Finalizers = newFinalizers
		return true
	}

	return false
}

// HasFinalizer checks if the pod has the specified finalizer
func (fp *FinalizerProcessor) HasFinalizer(pod *corev1.Pod, finalizer string) bool {
	for _, f := range pod.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

// HasSharedVolumeFinalizer checks if the pod has the shared volume finalizer
func (fp *FinalizerProcessor) HasSharedVolumeFinalizer(pod *corev1.Pod) bool {
	return fp.HasFinalizer(pod, types.SharedVolumePodFinalizer)
}

// AddSharedVolumeFinalizer adds the shared volume finalizer to the pod
func (fp *FinalizerProcessor) AddSharedVolumeFinalizer(pod *corev1.Pod) bool {
	return fp.AddFinalizer(pod, types.SharedVolumePodFinalizer)
}

// RemoveSharedVolumeFinalizer removes the shared volume finalizer from the pod
func (fp *FinalizerProcessor) RemoveSharedVolumeFinalizer(pod *corev1.Pod) bool {
	return fp.RemoveFinalizer(pod, types.SharedVolumePodFinalizer)
}

// ContainsString checks if a slice contains a specific string
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// RemoveString removes a specific string from a slice
func RemoveString(slice []string, s string) []string {
	result := []string{}
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// SanitizeFinalizers removes any empty or invalid finalizers
func (fp *FinalizerProcessor) SanitizeFinalizers(pod *corev1.Pod) {
	var cleanFinalizers []string

	for _, finalizer := range pod.Finalizers {
		finalizer = strings.TrimSpace(finalizer)
		if finalizer != "" && !ContainsString(cleanFinalizers, finalizer) {
			cleanFinalizers = append(cleanFinalizers, finalizer)
		}
	}

	pod.Finalizers = cleanFinalizers
}
