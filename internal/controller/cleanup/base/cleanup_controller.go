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

package base

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/cleanup/processors"
	"github.com/sharedvolume/shared-volume-controller/internal/controller/cleanup/types"
	"github.com/sharedvolume/shared-volume-controller/internal/controller/cleanup/validators"
)

// CleanupController is the base controller for pod cleanup operations
type CleanupController struct {
	client.Client
	Scheme                 *runtime.Scheme
	Config                 *types.CleanupConfig
	AnnotationProcessor    *processors.AnnotationProcessor
	FinalizerProcessor     *processors.FinalizerProcessor
	VolumeCleanupProcessor *processors.VolumeCleanupProcessor
	OrphanProcessor        *processors.OrphanProcessor
	Validator              *validators.CleanupValidator
}

// NewCleanupController creates a new cleanup controller with all processors
func NewCleanupController(client client.Client, scheme *runtime.Scheme) *CleanupController {
	config := &types.CleanupConfig{
		MaxRetries:                types.DefaultMaxRetries,
		ForceDeleteTimeoutSeconds: types.DefaultForceDeleteTimeout,
		CleanupBatchSize:          10,
	}

	controller := &CleanupController{
		Client:                 client,
		Scheme:                 scheme,
		Config:                 config,
		AnnotationProcessor:    processors.NewAnnotationProcessor(config),
		FinalizerProcessor:     processors.NewFinalizerProcessor(config),
		VolumeCleanupProcessor: processors.NewVolumeCleanupProcessor(client, config),
		OrphanProcessor:        processors.NewOrphanProcessor(client, config),
		Validator:              validators.NewCleanupValidator(),
	}

	return controller
}

// Reconcile handles pod deletion cleanup
func (r *CleanupController) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Defensive checks for nil processors
	if r.FinalizerProcessor == nil || r.AnnotationProcessor == nil || r.OrphanProcessor == nil || r.VolumeCleanupProcessor == nil {
		log.Error(nil, "CleanupController processors are nil - controller not properly initialized")
		return ctrl.Result{}, nil
	}

	// Fetch the Pod instance
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		// Pod not found, likely already deleted - don't log this as it's normal
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if pod has our finalizer
	if !r.FinalizerProcessor.HasFinalizer(&pod, types.SharedVolumePodFinalizer) {
		// Check if this pod uses SharedVolumes
		svRefs := r.AnnotationProcessor.ExtractSharedVolumeAnnotations(&pod)

		// Check if this is an orphaned pod by ReplicaSet (regardless of SharedVolume usage)
		if isOrphanedByReplicaSet := r.OrphanProcessor.IsOrphanedByReplicaSet(ctx, &pod); isOrphanedByReplicaSet {
			log.Info("Pod is orphaned by its ReplicaSet, cleaning up", "pod", pod.Name, "namespace", pod.Namespace, "phase", pod.Status.Phase)

			// Clean up SharedVolume resources if this pod uses them
			if len(svRefs) > 0 {
				if err := r.VolumeCleanupProcessor.CleanupSharedVolumeResources(ctx, &pod); err != nil {
					log.Error(err, "Failed to cleanup SharedVolume resources for orphaned pod")
					return ctrl.Result{}, err
				}
			}

			// Delete the orphaned pod
			if err := r.OrphanProcessor.DeleteOrphanedPod(ctx, &pod); err != nil {
				log.Error(err, "Failed to delete orphaned pod")
				return ctrl.Result{}, err
			}

			log.Info("Successfully cleaned up orphaned pod", "pod", pod.Name, "namespace", pod.Namespace)
			return ctrl.Result{}, nil
		}

		if len(svRefs) == 0 {
			// This pod doesn't use SharedVolumes and is not orphaned, ignore it silently
			return ctrl.Result{}, nil
		}

		// Pod uses SharedVolumes but doesn't have our finalizer, add it
		if pod.DeletionTimestamp == nil {
			log.Info("Adding finalizer to SharedVolume pod", "pod", pod.Name, "namespace", pod.Namespace)
			if err := r.FinalizerProcessor.AddFinalizer(ctx, r.Client, &pod, types.SharedVolumePodFinalizer); err != nil {
				log.Error(err, "Failed to add finalizer")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		// Pod is being deleted but doesn't have our finalizer, we still need to clean up
		log.Info("Pod is being deleted without our finalizer, performing cleanup anyway", "pod", pod.Name, "namespace", pod.Namespace)
	}

	// Check if pod is being deleted
	if pod.DeletionTimestamp != nil {
		log.Info("Pod is being deleted, starting SharedVolume cleanup", "pod", pod.Name, "namespace", pod.Namespace)

		// First, try to force delete the pod if it's stuck
		if err := r.FinalizerProcessor.ForceDeletePod(ctx, r.Client, &pod); err != nil {
			log.Error(err, "Failed to force delete stuck pod", "pod", pod.Name)
			// Continue with cleanup even if force delete failed
		}

		// Extract SharedVolume annotations and clean up PVC/PV
		if err := r.VolumeCleanupProcessor.CleanupSharedVolumeResources(ctx, &pod); err != nil {
			log.Error(err, "Failed to cleanup SharedVolume resources")
			return ctrl.Result{}, err
		}

		// Remove our finalizer to allow pod deletion
		log.Info("Attempting to remove finalizer from pod", "pod", pod.Name, "namespace", pod.Namespace, "finalizer", types.SharedVolumePodFinalizer)
		if err := r.FinalizerProcessor.RemoveFinalizer(ctx, r.Client, &pod, types.SharedVolumePodFinalizer); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}

		log.Info("Successfully removed finalizer from pod", "pod", pod.Name, "namespace", pod.Namespace, "finalizer", types.SharedVolumePodFinalizer)
		log.Info("Successfully cleaned up SharedVolume resources for pod", "pod", pod.Name, "namespace", pod.Namespace)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager
func (r *CleanupController) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			pod, ok := object.(*corev1.Pod)
			if !ok {
				return false
			}

			// Defensive check for nil processor
			if r.FinalizerProcessor == nil {
				// Log error but don't crash - just return false to skip this pod
				return false
			}

			// Watch all pods that use SharedVolumes (have our annotations)
			if pod.Annotations != nil {
				if sharedVolumeList, exists := pod.Annotations[types.SharedVolumeAnnotationKey]; exists && sharedVolumeList != "" {
					return true
				}
				if clusterSharedVolumeList, exists := pod.Annotations[types.ClusterSharedVolumeAnnotationKey]; exists && clusterSharedVolumeList != "" {
					return true
				}
			}

			// Also watch pods that already have our finalizer
			if r.FinalizerProcessor.HasFinalizer(pod, types.SharedVolumePodFinalizer) {
				return true
			}

			// Only watch ReplicaSet pods that are in specific namespaces that might contain SharedVolume resources
			// This reduces the scope significantly
			for _, owner := range pod.OwnerReferences {
				if owner.Kind == "ReplicaSet" && owner.APIVersion == "apps/v1" {
					// Only watch if the pod is in a namespace that might contain SharedVolume operations
					// or if it's explicitly marked for cleanup
					if pod.Namespace == types.ClusterSharedVolumeOperationNamespace ||
						pod.DeletionTimestamp != nil {
						return true
					}
				}
			}

			return false
		})).
		Named("pod-cleanup").
		Complete(r)
}
