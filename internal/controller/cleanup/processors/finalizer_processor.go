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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/cleanup/types"
)

// FinalizerProcessor handles finalizer management for cleanup operations
type FinalizerProcessor struct {
	config *types.CleanupConfig
}

// NewFinalizerProcessor creates a new finalizer processor
func NewFinalizerProcessor(config *types.CleanupConfig) *FinalizerProcessor {
	return &FinalizerProcessor{
		config: config,
	}
}

// HasFinalizer checks if the pod has the specified finalizer
func (p *FinalizerProcessor) HasFinalizer(pod *corev1.Pod, finalizer string) bool {
	for _, f := range pod.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

// AddFinalizer adds the specified finalizer to the pod
func (p *FinalizerProcessor) AddFinalizer(ctx context.Context, k8sClient client.Client, pod *corev1.Pod, finalizer string) error {
	log := logf.FromContext(ctx)

	// Get the latest version to avoid conflicts
	var latestPod corev1.Pod
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &latestPod); err != nil {
		log.Error(err, "Failed to get latest pod version for finalizer addition")
		return err
	}

	// Check if finalizer already exists
	for _, f := range latestPod.Finalizers {
		if f == finalizer {
			log.Info("Finalizer already exists on pod", "finalizer", finalizer)
			return nil
		}
	}

	// Add the finalizer
	latestPod.Finalizers = append(latestPod.Finalizers, finalizer)
	log.Info("Adding finalizer to pod", "pod", latestPod.Name, "namespace", latestPod.Namespace, "finalizer", finalizer)

	// Update the pod
	if err := k8sClient.Update(ctx, &latestPod); err != nil {
		log.Error(err, "Failed to update pod to add finalizer")
		return err
	}

	log.Info("Successfully added finalizer to pod", "finalizer", finalizer)
	return nil
}

// RemoveFinalizer removes the specified finalizer from the pod
func (p *FinalizerProcessor) RemoveFinalizer(ctx context.Context, k8sClient client.Client, pod *corev1.Pod, finalizer string) error {
	log := logf.FromContext(ctx)

	// Get the latest version to avoid conflicts
	var latestPod corev1.Pod
	if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &latestPod); err != nil {
		log.Error(err, "Failed to get latest pod version for finalizer removal")
		return err
	}

	// Check if finalizer exists
	var found bool
	var newFinalizers []string
	for _, f := range latestPod.Finalizers {
		if f != finalizer {
			newFinalizers = append(newFinalizers, f)
		} else {
			found = true
		}
	}

	if !found {
		log.Info("Finalizer not found on pod, nothing to remove", "finalizer", finalizer)
		return nil
	}

	// Update finalizers
	latestPod.Finalizers = newFinalizers
	log.Info("Updating pod to remove finalizer", "pod", latestPod.Name, "namespace", latestPod.Namespace, "finalizer", finalizer, "oldFinalizersCount", len(latestPod.Finalizers)+1, "newFinalizersCount", len(newFinalizers))

	// Update the pod
	if err := k8sClient.Update(ctx, &latestPod); err != nil {
		log.Error(err, "Failed to update pod to remove finalizer")
		return err
	}

	log.Info("Successfully updated pod to remove finalizer", "finalizer", finalizer)
	return nil
}

// ForceDeletePod attempts to delete a pod and removes all finalizers if deletion is stuck
func (p *FinalizerProcessor) ForceDeletePod(ctx context.Context, k8sClient client.Client, pod *corev1.Pod) error {
	log := logf.FromContext(ctx)

	// Check if pod is already being deleted and has finalizers
	if pod.DeletionTimestamp != nil && len(pod.Finalizers) > 0 {
		log.Info("Pod is stuck terminating, removing all finalizers",
			"name", pod.Name,
			"namespace", pod.Namespace,
			"finalizers", pod.Finalizers)

		// Get the latest version to avoid conflicts
		var latestPod corev1.Pod
		if err := k8sClient.Get(ctx, client.ObjectKeyFromObject(pod), &latestPod); err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("Pod already deleted", "name", pod.Name, "namespace", pod.Namespace)
				return nil
			}
			return err
		}

		// Remove all finalizers to force deletion
		latestPod.Finalizers = []string{}
		if err := k8sClient.Update(ctx, &latestPod); err != nil {
			log.Error(err, "Failed to remove finalizers from pod", "name", pod.Name)
			return err
		}
		log.Info("Successfully removed all finalizers from pod", "name", pod.Name)
	}

	return nil
}
