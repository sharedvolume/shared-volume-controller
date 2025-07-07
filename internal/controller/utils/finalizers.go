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

package utils

import (
	"context"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// FinalizerManager provides utilities for managing finalizers on Kubernetes objects
type FinalizerManager struct {
	Client client.Client
}

// NewFinalizerManager creates a new FinalizerManager instance
func NewFinalizerManager(client client.Client) *FinalizerManager {
	return &FinalizerManager{
		Client: client,
	}
}

// RemoveFinalizerWithRetry removes a finalizer from an object with retry logic
func (f *FinalizerManager) RemoveFinalizerWithRetry(ctx context.Context, obj client.Object, finalizerName string) error {
	log := logf.FromContext(ctx)

	// Maximum number of retries
	maxRetries := 3
	retryCount := 0

	for {
		// Get the latest version of the object before updating to prevent conflicts
		latestObj := obj.DeepCopyObject().(client.Object)
		if err := f.Client.Get(ctx, client.ObjectKeyFromObject(obj), latestObj); err != nil {
			if apierrors.IsNotFound(err) {
				// Object already deleted, nothing to do
				log.Info("Object already deleted, no need to remove finalizer", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
				return nil
			}
			log.Error(err, "Failed to get latest object before finalizer removal", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
			return err
		}

		// Check if the finalizer is still present
		if !ContainsString(latestObj.GetFinalizers(), finalizerName) {
			log.Info("Finalizer already removed", "finalizer", finalizerName, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
			// Update the original object reference with the latest version
			obj.SetResourceVersion(latestObj.GetResourceVersion())
			obj.SetFinalizers(latestObj.GetFinalizers())
			return nil
		}

		// Remove the finalizer from the latest version
		finalizers := RemoveString(latestObj.GetFinalizers(), finalizerName)
		latestObj.SetFinalizers(finalizers)

		// Update the object
		if err := f.Client.Update(ctx, latestObj); err != nil {
			if apierrors.IsConflict(err) && retryCount < maxRetries {
				retryCount++
				log.Info("Conflict detected while removing finalizer, retrying", "retryCount", retryCount, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
				time.Sleep(time.Millisecond * 100 * time.Duration(retryCount)) // Backoff with each retry
				continue
			}
			if apierrors.IsNotFound(err) {
				// Object was deleted during our retry, which is fine
				log.Info("Object was deleted during finalizer removal", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
				return nil
			}
			log.Error(err, "Failed to remove finalizer after retries", "finalizer", finalizerName, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
			return err
		}

		// Success - update our reference with the latest version
		obj.SetResourceVersion(latestObj.GetResourceVersion())
		obj.SetFinalizers(latestObj.GetFinalizers())
		log.Info("Successfully removed finalizer", "finalizer", finalizerName, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
		return nil
	}
}

// AddFinalizerIfNotPresent adds a finalizer to an object if it's not already present
func (f *FinalizerManager) AddFinalizerIfNotPresent(ctx context.Context, obj client.Object, finalizerName string) error {
	log := logf.FromContext(ctx)

	if ContainsString(obj.GetFinalizers(), finalizerName) {
		log.V(1).Info("Finalizer already present", "finalizer", finalizerName, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
		return nil
	}

	// Add the finalizer
	finalizers := append(obj.GetFinalizers(), finalizerName)
	obj.SetFinalizers(finalizers)

	// Update the object
	if err := f.Client.Update(ctx, obj); err != nil {
		log.Error(err, "Failed to add finalizer", "finalizer", finalizerName, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
		return err
	}

	log.Info("Successfully added finalizer", "finalizer", finalizerName, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
	return nil
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
