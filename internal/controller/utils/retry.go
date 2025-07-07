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

// RetryManager provides utilities for retry operations
type RetryManager struct {
	Client client.Client
}

// NewRetryManager creates a new RetryManager instance
func NewRetryManager(client client.Client) *RetryManager {
	return &RetryManager{
		Client: client,
	}
}

// UpdateStatusWithRetry updates the status of any client.Object with retry and conflict handling
func (r *RetryManager) UpdateStatusWithRetry(ctx context.Context, obj client.Object, updateFn func(client.Object)) error {
	log := logf.FromContext(ctx)

	// Maximum number of retries
	maxRetries := 3
	retryCount := 0

	for {
		// Get the latest version of the object before updating to prevent conflicts
		latestObj := obj.DeepCopyObject().(client.Object)
		if err := r.Client.Get(ctx, client.ObjectKeyFromObject(obj), latestObj); err != nil {
			log.Error(err, "Failed to get latest object before status update", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
			return err
		}

		// Apply the status update function to the latest object
		updateFn(latestObj)

		// Update the status on the latest version of the object
		if err := r.Client.Status().Update(ctx, latestObj); err != nil {
			if apierrors.IsConflict(err) && retryCount < maxRetries {
				retryCount++
				log.Info("Conflict detected while updating status, retrying", "retryCount", retryCount, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
				time.Sleep(time.Millisecond * 100 * time.Duration(retryCount)) // Backoff with each retry
				continue
			}
			log.Error(err, "Failed to update object status after retries", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
			return err
		}

		// Success - update our reference with the latest version
		obj.SetResourceVersion(latestObj.GetResourceVersion())
		log.V(1).Info("Successfully updated object status", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
		return nil
	}
}

// RetryOperation executes a function with retry logic
func (r *RetryManager) RetryOperation(ctx context.Context, operation func() error, maxRetries int, backoffBase time.Duration) error {
	log := logf.FromContext(ctx)

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := backoffBase * time.Duration(attempt)
			log.V(1).Info("Retrying operation", "attempt", attempt, "backoff", backoff)
			time.Sleep(backoff)
		}

		if err := operation(); err != nil {
			lastErr = err
			if apierrors.IsConflict(err) && attempt < maxRetries {
				continue
			}
			if attempt == maxRetries {
				log.Error(err, "Operation failed after all retries", "attempts", attempt+1)
				return err
			}
			continue
		}

		// Success
		if attempt > 0 {
			log.Info("Operation succeeded after retry", "attempts", attempt+1)
		}
		return nil
	}

	return lastErr
}

// ExponentialBackoff provides exponential backoff retry logic
func (r *RetryManager) ExponentialBackoff(ctx context.Context, operation func() error, maxRetries int, initialDelay time.Duration) error {
	log := logf.FromContext(ctx)

	var lastErr error
	delay := initialDelay

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.V(1).Info("Retrying operation with exponential backoff", "attempt", attempt, "delay", delay)
			time.Sleep(delay)
			delay *= 2 // Double the delay for next attempt
		}

		if err := operation(); err != nil {
			lastErr = err
			if attempt == maxRetries {
				log.Error(err, "Operation failed after all retries with exponential backoff", "attempts", attempt+1)
				return err
			}
			continue
		}

		// Success
		if attempt > 0 {
			log.Info("Operation succeeded after exponential backoff retry", "attempts", attempt+1)
		}
		return nil
	}

	return lastErr
}
