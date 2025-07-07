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

	"github.com/sharedvolume/shared-volume-controller/internal/controller/cleanup/types"
)

// OrphanProcessor handles orphaned pod detection and cleanup
type OrphanProcessor struct {
	client client.Client
	config *types.CleanupConfig
}

// NewOrphanProcessor creates a new orphan processor
func NewOrphanProcessor(client client.Client, config *types.CleanupConfig) *OrphanProcessor {
	return &OrphanProcessor{
		client: client,
		config: config,
	}
}

// IsOrphanedByReplicaSet checks if a pod was created by a ReplicaSet that no longer exists
func (p *OrphanProcessor) IsOrphanedByReplicaSet(ctx context.Context, pod *corev1.Pod) bool {
	// TODO: Implement orphan detection logic from the original controller
	// This is a placeholder for now
	return false
}

// DeleteOrphanedPod deletes an orphaned pod
func (p *OrphanProcessor) DeleteOrphanedPod(ctx context.Context, pod *corev1.Pod) error {
	// Delete the orphaned pod
	if err := p.client.Delete(ctx, pod); err != nil {
		if apierrors.IsNotFound(err) {
			// Pod was already deleted, this is expected in a race condition
			return nil
		}
		return err
	}
	return nil
}
