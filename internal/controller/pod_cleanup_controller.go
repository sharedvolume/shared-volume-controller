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

package controller

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

const (
	SharedVolumePodFinalizer  = "sharedvolume.sv/pod-cleanup"
	SharedVolumeAnnotationKey = "sharedvolume.sv"
)

// PodCleanupReconciler reconciles Pod deletions for SharedVolume cleanup
type PodCleanupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;delete;update;patch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumes,verbs=get;list;watch;delete;update;patch
// +kubebuilder:rbac:groups="",resources=persistentvolumes/finalizers,verbs=update
// +kubebuilder:rbac:groups="apps",resources=replicasets,verbs=get;list;watch

// Reconcile handles pod deletion cleanup
func (r *PodCleanupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the Pod instance
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		// Pod not found, likely already deleted
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Check if pod has our finalizer
	if !r.hasFinalizer(&pod, SharedVolumePodFinalizer) {
		// Check if this pod uses SharedVolumes
		svRefs := r.extractSharedVolumeAnnotations(&pod)

		// Check if this is an orphaned pod by ReplicaSet (regardless of SharedVolume usage)
		if isOrphanedByReplicaSet := r.isOrphanedByReplicaSet(ctx, &pod); isOrphanedByReplicaSet {
			log.Info("Pod is orphaned by its ReplicaSet, cleaning up", "pod", pod.Name, "namespace", pod.Namespace, "phase", pod.Status.Phase)

			// Clean up SharedVolume resources if this pod uses them
			if len(svRefs) > 0 {
				if err := r.cleanupSharedVolumeResources(ctx, &pod); err != nil {
					log.Error(err, "Failed to cleanup SharedVolume resources for orphaned pod")
					return ctrl.Result{}, err
				}
			}
			// Delete the orphaned pod
			if err := r.Delete(ctx, &pod); err != nil {
				if apierrors.IsNotFound(err) {
					// Pod was already deleted, this is expected in a race condition
					log.Info("Orphaned pod was already deleted", "pod", pod.Name, "namespace", pod.Namespace)
					return ctrl.Result{}, nil
				}
				log.Error(err, "Failed to delete orphaned pod", "pod", pod.Name)
				return ctrl.Result{}, err
			}

			log.Info("Successfully cleaned up orphaned pod", "pod", pod.Name, "namespace", pod.Namespace)
			return ctrl.Result{}, nil
		}

		if len(svRefs) == 0 {
			// This pod doesn't use SharedVolumes and is not orphaned, ignore it
			return ctrl.Result{}, nil
		}

		// Pod uses SharedVolumes but doesn't have our finalizer, add it
		if pod.DeletionTimestamp == nil {
			log.Info("Adding finalizer to SharedVolume pod", "pod", pod.Name, "namespace", pod.Namespace)
			if err := r.addFinalizer(ctx, &pod, SharedVolumePodFinalizer); err != nil {
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
		if err := r.forceDeletePod(ctx, &pod); err != nil {
			log.Error(err, "Failed to force delete stuck pod", "pod", pod.Name)
			// Continue with cleanup even if force delete failed
		}

		// Extract SharedVolume annotations and clean up PVC/PV
		if err := r.cleanupSharedVolumeResources(ctx, &pod); err != nil {
			log.Error(err, "Failed to cleanup SharedVolume resources")
			return ctrl.Result{}, err
		}

		// Remove our finalizer to allow pod deletion
		log.Info("Attempting to remove finalizer from pod", "pod", pod.Name, "namespace", pod.Namespace, "finalizer", SharedVolumePodFinalizer)
		if err := r.removeFinalizer(ctx, &pod, SharedVolumePodFinalizer); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}

		log.Info("Successfully removed finalizer from pod", "pod", pod.Name, "namespace", pod.Namespace, "finalizer", SharedVolumePodFinalizer)
		log.Info("Successfully cleaned up SharedVolume resources for pod", "pod", pod.Name, "namespace", pod.Namespace)
	}

	return ctrl.Result{}, nil
}

// hasFinalizer checks if the pod has the specified finalizer
func (r *PodCleanupReconciler) hasFinalizer(pod *corev1.Pod, finalizer string) bool {
	for _, f := range pod.Finalizers {
		if f == finalizer {
			return true
		}
	}
	return false
}

// removeFinalizer removes the specified finalizer from the pod
func (r *PodCleanupReconciler) removeFinalizer(ctx context.Context, pod *corev1.Pod, finalizer string) error {
	log := logf.FromContext(ctx)

	// Get the latest version to avoid conflicts
	var latestPod corev1.Pod
	if err := r.Get(ctx, client.ObjectKeyFromObject(pod), &latestPod); err != nil {
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
	if err := r.Update(ctx, &latestPod); err != nil {
		log.Error(err, "Failed to update pod to remove finalizer")
		return err
	}

	log.Info("Successfully updated pod to remove finalizer", "finalizer", finalizer)
	return nil
}

// addFinalizer adds the specified finalizer to the pod
func (r *PodCleanupReconciler) addFinalizer(ctx context.Context, pod *corev1.Pod, finalizer string) error {
	log := logf.FromContext(ctx)

	// Get the latest version to avoid conflicts
	var latestPod corev1.Pod
	if err := r.Get(ctx, client.ObjectKeyFromObject(pod), &latestPod); err != nil {
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
	if err := r.Update(ctx, &latestPod); err != nil {
		log.Error(err, "Failed to update pod to add finalizer")
		return err
	}

	log.Info("Successfully added finalizer to pod", "finalizer", finalizer)
	return nil
}

// SharedVolumeRef represents a SharedVolume reference with namespace and name
type SharedVolumeRef struct {
	Namespace string
	Name      string
}

// extractSharedVolumeAnnotations extracts SharedVolume references from pod annotations
func (r *PodCleanupReconciler) extractSharedVolumeAnnotations(pod *corev1.Pod) []SharedVolumeRef {
	var sharedVolumes []SharedVolumeRef

	if pod.Annotations == nil {
		return sharedVolumes
	}

	// Look for SharedVolume annotation: "sharedvolume.sv": "sv1,sv2,sv3"
	if sharedVolumeList, exists := pod.Annotations[SharedVolumeAnnotationKey]; exists && sharedVolumeList != "" {
		// Split by comma and trim spaces
		svNames := strings.Split(sharedVolumeList, ",")
		for _, svName := range svNames {
			svName = strings.TrimSpace(svName)
			if svName != "" {
				// All SharedVolumes are in the same namespace as the pod
				sharedVolumes = append(sharedVolumes, SharedVolumeRef{
					Namespace: pod.Namespace,
					Name:      svName,
				})
			}
		}
	}

	return sharedVolumes
}

// cleanupSharedVolumeResources cleans up PVC and PV for each SharedVolume used by the pod
func (r *PodCleanupReconciler) cleanupSharedVolumeResources(ctx context.Context, pod *corev1.Pod) error {
	log := logf.FromContext(ctx)

	// Extract SharedVolume references from annotations
	svRefs := r.extractSharedVolumeAnnotations(pod)
	if len(svRefs) == 0 {
		log.Info("No SharedVolume annotations found in pod", "pod", pod.Name)
		return nil
	}

	log.Info("Starting comprehensive cleanup for pod resources",
		"pod", pod.Name,
		"namespace", pod.Namespace,
		"volumeCount", len(svRefs))

	for _, svRef := range svRefs {
		log.Info("Cleaning up resources for SharedVolume",
			"sharedVolume", svRef.Name,
			"namespace", svRef.Namespace,
			"pod", pod.Name)

		// Get the SharedVolume to obtain its ReferenceValue
		sv := &svv1alpha1.SharedVolume{}
		if err := r.Get(ctx, types.NamespacedName{
			Namespace: svRef.Namespace,
			Name:      svRef.Name,
		}, sv); err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("SharedVolume not found, skipping cleanup",
					"sharedVolume", svRef.Name,
					"namespace", svRef.Namespace)
				continue
			}
			log.Error(err, "Failed to get SharedVolume",
				"sharedVolume", svRef.Name,
				"namespace", svRef.Namespace)
			return err
		}

		referenceValue := sv.Spec.ReferenceValue
		resourceNamespace := pod.Namespace // pod's namespace for SharedVolume

		// Clean up pod-specific PVC and PV created by the webhook for this pod
		if err := r.cleanupPodSpecificResourcesByRef(ctx, svRef, referenceValue, resourceNamespace); err != nil {
			log.Error(err, "Failed to cleanup pod-specific resources",
				"volume", svRef.Name,
				"pod", pod.Name)
			return err
		}

		// NOTE: Removed cleanupOrphanedResources() to prevent deleting shared infrastructure
		// Only clean up resources specifically created for this pod
	}

	log.Info("Completed comprehensive cleanup for pod resources", "pod", pod.Name)
	return nil
}

// cleanupPodSpecificResources cleans up the PVC and PV only if no other pods are using them
func (r *PodCleanupReconciler) cleanupPodSpecificResources(ctx context.Context, sv *svv1alpha1.SharedVolume, podNamespace string) error {
	log := logf.FromContext(ctx)

	// Generate the PVC and PV names using the same logic as the webhook
	// The webhook uses the same naming pattern for both PVC and PV: "{referenceValue}-{namespace}"
	pvcName := fmt.Sprintf("%s-%s", sv.Spec.ReferenceValue, podNamespace)
	pvName := fmt.Sprintf("%s-%s", sv.Spec.ReferenceValue, podNamespace)

	// Check if any other pods in the namespace are still using this SharedVolume
	stillInUse, err := r.isSharedVolumeStillInUse(ctx, sv, podNamespace)
	if err != nil {
		log.Error(err, "Failed to check if SharedVolume is still in use", "sharedVolume", sv.Name)
		return err
	}

	if stillInUse {
		log.Info("SharedVolume is still in use by other pods, skipping PVC/PV deletion",
			"sharedVolume", sv.Name,
			"namespace", sv.Namespace,
			"pvcName", pvcName,
			"pvName", pvName)
		return nil
	}

	log.Info("No other pods using SharedVolume, proceeding with PVC/PV cleanup",
		"sharedVolume", sv.Name,
		"namespace", sv.Namespace,
		"pvcName", pvcName,
		"pvName", pvName)

	// Delete PVC with finalizer removal if stuck
	if err := r.forceDeletePVC(ctx, pvcName, podNamespace); err != nil {
		log.Error(err, "Failed to force delete PVC", "name", pvcName)
		return err
	}

	// Delete PV with finalizer removal if stuck
	if err := r.forceDeletePV(ctx, pvName); err != nil {
		log.Error(err, "Failed to force delete PV", "name", pvName)
		return err
	}

	log.Info("Successfully cleaned up PVC and PV",
		"pvcName", pvcName,
		"pvName", pvName,
		"namespace", podNamespace)

	return nil
}

// cleanupPodSpecificResourcesByRef cleans up the PVC and PV only if no other pods are using them
func (r *PodCleanupReconciler) cleanupPodSpecificResourcesByRef(ctx context.Context, svRef SharedVolumeRef, referenceValue, resourceNamespace string) error {
	log := logf.FromContext(ctx)

	// Generate the PVC and PV names using the same logic as the webhook
	// The webhook uses the same naming pattern for both PVC and PV: "{referenceValue}-{namespace}"
	pvcName := fmt.Sprintf("%s-%s", referenceValue, resourceNamespace)
	pvName := fmt.Sprintf("%s-%s", referenceValue, resourceNamespace)

	// Check if any other pods are still using this volume
	stillInUse, err := r.isVolumeStillInUseByRef(ctx, svRef)
	if err != nil {
		log.Error(err, "Failed to check if volume is still in use",
			"volume", svRef.Name)
		return err
	}

	if stillInUse {
		log.Info("Volume is still in use by other pods, skipping PVC/PV deletion",
			"volume", svRef.Name,
			"namespace", svRef.Namespace,
			"pvcName", pvcName,
			"pvName", pvName)
		return nil
	}

	log.Info("No other pods using volume, proceeding with PVC/PV cleanup",
		"volume", svRef.Name,
		"namespace", svRef.Namespace,
		"pvcName", pvcName,
		"pvName", pvName)

	// Delete PVC with finalizer removal if stuck
	if err := r.forceDeletePVC(ctx, pvcName, resourceNamespace); err != nil {
		log.Error(err, "Failed to force delete PVC", "name", pvcName)
		return err
	}

	// Delete PV with finalizer removal if stuck
	if err := r.forceDeletePV(ctx, pvName); err != nil {
		log.Error(err, "Failed to force delete PV", "name", pvName)
		return err
	}

	log.Info("Successfully cleaned up PVC and PV",
		"pvcName", pvcName,
		"pvName", pvName,
		"namespace", resourceNamespace)

	return nil
}

// isSharedVolumeStillInUse checks if any other pods in the namespace are still using this SharedVolume
func (r *PodCleanupReconciler) isSharedVolumeStillInUse(ctx context.Context, sv *svv1alpha1.SharedVolume, namespace string) (bool, error) {
	log := logf.FromContext(ctx)

	// List all pods in the namespace
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(namespace)); err != nil {
		log.Error(err, "Failed to list pods in namespace", "namespace", namespace)
		return false, err
	}

	// Check each pod for SharedVolume usage
	for _, pod := range podList.Items {
		if r.isPodUsingSharedVolume(&pod, sv) {
			log.Info("Found pod still using SharedVolume",
				"pod", pod.Name,
				"namespace", pod.Namespace,
				"sharedVolume", sv.Name,
				"sharedVolumeNamespace", sv.Namespace)
			return true, nil
		}
	}

	log.Info("No active pods found using SharedVolume",
		"sharedVolume", sv.Name,
		"namespace", sv.Namespace,
		"checkedNamespace", namespace)
	return false, nil
}

// isPodUsingSharedVolume checks if a pod is using the specified SharedVolume
func (r *PodCleanupReconciler) isPodUsingSharedVolume(pod *corev1.Pod, sv *svv1alpha1.SharedVolume) bool {
	// Skip pods that are being deleted or already terminated
	if pod.DeletionTimestamp != nil {
		return false
	}

	if pod.Annotations == nil {
		return false
	}

	// Check the new annotation format: "sharedvolume.sv": "sv1,sv2,sv3"
	if sharedVolumeList, exists := pod.Annotations[SharedVolumeAnnotationKey]; exists && sharedVolumeList != "" {
		svNames := strings.Split(sharedVolumeList, ",")
		for _, svName := range svNames {
			svName = strings.TrimSpace(svName)
			if svName != "" {
				// All SharedVolumes are in the same namespace as the pod
				if pod.Namespace == sv.Namespace && svName == sv.Name {
					return true
				}
			}
		}
	}
	return false
}

// isVolumeStillInUseByRef checks if any other pods are still using this SharedVolume
func (r *PodCleanupReconciler) isVolumeStillInUseByRef(ctx context.Context, svRef SharedVolumeRef) (bool, error) {
	log := logf.FromContext(ctx)

	// Check only the specific namespace for SharedVolume
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList, client.InNamespace(svRef.Namespace)); err != nil {
		log.Error(err, "Failed to list pods in namespace", "namespace", svRef.Namespace)
		return false, err
	}

	// Check each pod for volume usage
	for _, pod := range podList.Items {
		if r.isPodUsingVolumeByRef(&pod, svRef) {
			log.Info("Found pod still using volume",
				"pod", pod.Name,
				"namespace", pod.Namespace,
				"volume", svRef.Name)
			return true, nil
		}
	}

	log.Info("No active pods found using volume",
		"volume", svRef.Name,
		"namespace", svRef.Namespace)
	return false, nil
}

// isPodUsingVolumeByRef checks if a pod is using the specified SharedVolume
func (r *PodCleanupReconciler) isPodUsingVolumeByRef(pod *corev1.Pod, svRef SharedVolumeRef) bool {
	// Skip pods that are being deleted or already terminated
	if pod.DeletionTimestamp != nil {
		return false
	}

	if pod.Annotations == nil {
		return false
	}

	// Check for SharedVolume annotation: "sharedvolume.sv": "sv1,sv2,sv3"
	if sharedVolumeList, exists := pod.Annotations[SharedVolumeAnnotationKey]; exists && sharedVolumeList != "" {
		svNames := strings.Split(sharedVolumeList, ",")
		for _, svName := range svNames {
			svName = strings.TrimSpace(svName)
			if svName != "" {
				// All SharedVolumes are in the same namespace as the pod
				if pod.Namespace == svRef.Namespace && svName == svRef.Name {
					return true
				}
			}
		}
	}
	return false
}

// forceDeletePVC attempts to delete a PVC and removes finalizers if deletion is stuck
func (r *PodCleanupReconciler) forceDeletePVC(ctx context.Context, pvcName, namespace string) error {
	log := logf.FromContext(ctx)

	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      pvcName,
		Namespace: namespace,
	}, pvc); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PVC not found, already deleted", "name", pvcName, "namespace", namespace)
			return nil
		}
		log.Error(err, "Failed to get PVC", "name", pvcName)
		return err
	}

	log.Info("Deleting PVC", "name", pvcName, "namespace", namespace)

	// First check if PVC has problematic finalizers and remove them proactively
	if len(pvc.Finalizers) > 0 {
		log.Info("PVC has finalizers, removing them before deletion to prevent getting stuck",
			"name", pvcName,
			"namespace", namespace,
			"finalizers", pvc.Finalizers)

		if err := r.removePVCFinalizersWithRetry(ctx, pvc); err != nil {
			log.Error(err, "Failed to proactively remove PVC finalizers", "name", pvcName)
			// Continue with deletion attempt even if finalizer removal failed
		}
	}

	// Now try normal deletion
	if err := r.Delete(ctx, pvc); err != nil {
		if apierrors.IsNotFound(err) {
			// PVC was already deleted
			log.Info("PVC was already deleted", "name", pvcName)
			return nil
		}
		log.Error(err, "Failed to delete PVC normally", "name", pvcName)
		return err
	}

	// Check if PVC is stuck in terminating state due to finalizers
	// Give it a moment to delete normally first
	var updatedPVC corev1.PersistentVolumeClaim
	if err := r.Get(ctx, types.NamespacedName{
		Name:      pvcName,
		Namespace: namespace,
	}, &updatedPVC); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PVC successfully deleted", "name", pvcName, "namespace", namespace)
			return nil
		}
		return err
	}

	// PVC still exists, check if it's terminating and has finalizers
	if updatedPVC.DeletionTimestamp != nil && len(updatedPVC.Finalizers) > 0 {
		log.Info("PVC is stuck terminating, removing finalizers",
			"name", pvcName,
			"namespace", namespace,
			"finalizers", updatedPVC.Finalizers)

		// Remove all finalizers to force deletion with retry
		if err := r.removePVCFinalizersWithRetry(ctx, &updatedPVC); err != nil {
			log.Error(err, "Failed to remove finalizers from PVC", "name", pvcName)
			return err
		}
	}

	return nil
}

// forceDeletePV attempts to delete a PV and removes finalizers if deletion is stuck
func (r *PodCleanupReconciler) forceDeletePV(ctx context.Context, pvName string) error {
	log := logf.FromContext(ctx)

	pv := &corev1.PersistentVolume{}
	if err := r.Get(ctx, types.NamespacedName{Name: pvName}, pv); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PV not found, already deleted", "name", pvName)
			return nil
		}
		log.Error(err, "Failed to get PV", "name", pvName)
		return err
	}

	log.Info("Deleting PV", "name", pvName)

	// First check if PV has CSI or other problematic finalizers and remove them proactively
	if len(pv.Finalizers) > 0 {
		log.Info("PV has finalizers, removing them before deletion to prevent getting stuck",
			"name", pvName,
			"finalizers", pv.Finalizers)

		if err := r.removePVFinalizersWithRetry(ctx, pv); err != nil {
			log.Error(err, "Failed to proactively remove PV finalizers", "name", pvName)
			// Continue with deletion attempt even if finalizer removal failed
		}
	}

	// Now try normal deletion
	if err := r.Delete(ctx, pv); err != nil {
		if apierrors.IsNotFound(err) {
			// PV was already deleted
			log.Info("PV was already deleted", "name", pvName)
			return nil
		}
		log.Error(err, "Failed to delete PV normally", "name", pvName)
		return err
	}

	// Check if PV is stuck in terminating state due to finalizers
	// Give it a moment to delete normally first
	var updatedPV corev1.PersistentVolume
	if err := r.Get(ctx, types.NamespacedName{Name: pvName}, &updatedPV); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PV successfully deleted", "name", pvName)
			return nil
		}
		return err
	}

	// PV still exists, check if it's terminating and has finalizers
	if updatedPV.DeletionTimestamp != nil && len(updatedPV.Finalizers) > 0 {
		log.Info("PV is stuck terminating, removing finalizers",
			"name", pvName,
			"finalizers", updatedPV.Finalizers)

		// Remove all finalizers to force deletion with retry
		if err := r.removePVFinalizersWithRetry(ctx, &updatedPV); err != nil {
			log.Error(err, "Failed to remove finalizers from PV", "name", pvName)
			return err
		}
	}

	return nil
}

// forceDeletePod attempts to delete a pod and removes all finalizers if deletion is stuck
func (r *PodCleanupReconciler) forceDeletePod(ctx context.Context, pod *corev1.Pod) error {
	log := logf.FromContext(ctx)

	// Check if pod is already being deleted and has finalizers
	if pod.DeletionTimestamp != nil && len(pod.Finalizers) > 0 {
		log.Info("Pod is stuck terminating, removing all finalizers",
			"name", pod.Name,
			"namespace", pod.Namespace,
			"finalizers", pod.Finalizers)

		// Get the latest version to avoid conflicts
		var latestPod corev1.Pod
		if err := r.Get(ctx, client.ObjectKeyFromObject(pod), &latestPod); err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("Pod already deleted", "name", pod.Name, "namespace", pod.Namespace)
				return nil
			}
			return err
		}

		// Remove all finalizers to force deletion
		latestPod.Finalizers = []string{}
		if err := r.Update(ctx, &latestPod); err != nil {
			log.Error(err, "Failed to remove finalizers from pod", "name", pod.Name)
			return err
		}
		log.Info("Successfully removed all finalizers from pod", "name", pod.Name)
	}

	return nil
}

// removePVCFinalizersWithRetry removes finalizers from a PVC with retry logic to handle conflicts
func (r *PodCleanupReconciler) removePVCFinalizersWithRetry(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
	log := logf.FromContext(ctx)

	maxRetries := 5 // Increased retries for stubborn finalizers
	retryCount := 0

	for {
		// Get the latest version of the PVC
		var latestPVC corev1.PersistentVolumeClaim
		if err := r.Get(ctx, types.NamespacedName{
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
		}, &latestPVC); err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("PVC already deleted during finalizer removal", "name", pvc.Name, "namespace", pvc.Namespace)
				return nil
			}
			return err
		}

		// If no finalizers, we're done
		if len(latestPVC.Finalizers) == 0 {
			log.Info("PVC has no finalizers to remove", "name", pvc.Name, "namespace", pvc.Namespace)
			return nil
		}

		log.Info("Attempting to remove PVC finalizers",
			"name", pvc.Name,
			"namespace", pvc.Namespace,
			"attempt", retryCount+1,
			"finalizers", latestPVC.Finalizers)

		// Remove all finalizers
		latestPVC.Finalizers = []string{}

		// Try to update
		if err := r.Update(ctx, &latestPVC); err != nil {
			if apierrors.IsConflict(err) && retryCount < maxRetries {
				retryCount++
				log.Info("Conflict updating PVC finalizers, retrying",
					"name", pvc.Name,
					"namespace", pvc.Namespace,
					"retry", retryCount,
					"maxRetries", maxRetries)
				continue
			}

			// If regular update fails and we still have retries, try a more aggressive approach
			if retryCount < maxRetries {
				retryCount++
				log.Info("Regular update failed, trying strategic patch to force remove finalizers",
					"name", pvc.Name,
					"namespace", pvc.Namespace,
					"retry", retryCount,
					"error", err.Error())

				// Use strategic patch to set finalizers to null
				if patchErr := r.forceRemovePVCFinalizersWithPatch(ctx, pvc.Name, pvc.Namespace); patchErr != nil {
					log.Error(patchErr, "Strategic patch also failed", "name", pvc.Name, "namespace", pvc.Namespace)
					if retryCount >= maxRetries {
						return patchErr
					}
					continue
				}

				log.Info("Strategic patch succeeded in removing PVC finalizers", "name", pvc.Name, "namespace", pvc.Namespace)
				return nil
			}

			return err
		}

		log.Info("Successfully removed finalizers from PVC", "name", pvc.Name, "namespace", pvc.Namespace)
		return nil
	}
}

// removePVFinalizersWithRetry removes finalizers from a PV with retry logic to handle conflicts
func (r *PodCleanupReconciler) removePVFinalizersWithRetry(ctx context.Context, pv *corev1.PersistentVolume) error {
	log := logf.FromContext(ctx)

	maxRetries := 5 // Increased retries for stubborn CSI finalizers
	retryCount := 0

	for {
		// Get the latest version of the PV
		var latestPV corev1.PersistentVolume
		if err := r.Get(ctx, types.NamespacedName{Name: pv.Name}, &latestPV); err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("PV already deleted during finalizer removal", "name", pv.Name)
				return nil
			}
			return err
		}

		// If no finalizers, we're done
		if len(latestPV.Finalizers) == 0 {
			log.Info("PV has no finalizers to remove", "name", pv.Name)
			return nil
		}

		log.Info("Attempting to remove PV finalizers",
			"name", pv.Name,
			"attempt", retryCount+1,
			"finalizers", latestPV.Finalizers)

		// Remove all finalizers
		latestPV.Finalizers = []string{}

		// Try to update
		if err := r.Update(ctx, &latestPV); err != nil {
			if apierrors.IsConflict(err) && retryCount < maxRetries {
				retryCount++
				log.Info("Conflict updating PV finalizers, retrying",
					"name", pv.Name,
					"retry", retryCount,
					"maxRetries", maxRetries)
				continue
			}

			// If regular update fails and we still have retries, try a more aggressive approach
			if retryCount < maxRetries {
				retryCount++
				log.Info("Regular update failed, trying strategic patch to force remove finalizers",
					"name", pv.Name,
					"retry", retryCount,
					"error", err.Error())

				// Use strategic patch to set finalizers to null
				if patchErr := r.forceRemovePVFinalizersWithPatch(ctx, pv.Name); patchErr != nil {
					log.Error(patchErr, "Strategic patch also failed", "name", pv.Name)
					if retryCount >= maxRetries {
						return patchErr
					}
					continue
				}

				log.Info("Strategic patch succeeded in removing PV finalizers", "name", pv.Name)
				return nil
			}

			return err
		}

		log.Info("Successfully removed finalizers from PV", "name", pv.Name)
		return nil
	}
}

// forceRemovePVFinalizersWithPatch uses strategic patch to aggressively remove PV finalizers
func (r *PodCleanupReconciler) forceRemovePVFinalizersWithPatch(ctx context.Context, pvName string) error {
	log := logf.FromContext(ctx)

	// Create a patch that sets finalizers to null (more aggressive than empty array)
	patch := []byte(`{"metadata":{"finalizers":null}}`)

	log.Info("Using strategic patch to force remove PV finalizers", "name", pvName)

	// Apply the patch
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
	}

	if err := r.Patch(ctx, pv, client.RawPatch(types.MergePatchType, patch)); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PV not found during strategic patch, already deleted", "name", pvName)
			return nil
		}
		return err
	}

	log.Info("Successfully applied strategic patch to remove PV finalizers", "name", pvName)
	return nil
}

// forceRemovePVCFinalizersWithPatch uses strategic patch to aggressively remove PVC finalizers
func (r *PodCleanupReconciler) forceRemovePVCFinalizersWithPatch(ctx context.Context, pvcName, namespace string) error {
	log := logf.FromContext(ctx)

	// Create a patch that sets finalizers to null (more aggressive than empty array)
	patch := []byte(`{"metadata":{"finalizers":null}}`)

	log.Info("Using strategic patch to force remove PVC finalizers", "name", pvcName, "namespace", namespace)

	// Apply the patch
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
	}

	if err := r.Patch(ctx, pvc, client.RawPatch(types.MergePatchType, patch)); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PVC not found during strategic patch, already deleted", "name", pvcName, "namespace", namespace)
			return nil
		}
		return err
	}

	log.Info("Successfully applied strategic patch to remove PVC finalizers", "name", pvcName, "namespace", namespace)
	return nil
}

// isOrphanedByReplicaSet checks if a pod was created by a ReplicaSet that no longer exists
func (r *PodCleanupReconciler) isOrphanedByReplicaSet(ctx context.Context, pod *corev1.Pod) bool {
	log := logf.FromContext(ctx)

	// Check if the pod has an owner reference to a ReplicaSet
	var replicaSetOwner *metav1.OwnerReference
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "ReplicaSet" && owner.APIVersion == "apps/v1" {
			replicaSetOwner = &owner
			break
		}
	}

	// If no ReplicaSet owner reference, it's not created by a ReplicaSet
	if replicaSetOwner == nil {
		return false
	}

	// Check if the ReplicaSet still exists
	var replicaSet appsv1.ReplicaSet
	err := r.Get(ctx, types.NamespacedName{
		Name:      replicaSetOwner.Name,
		Namespace: pod.Namespace,
	}, &replicaSet)

	if apierrors.IsNotFound(err) {
		log.Info("ReplicaSet owner not found, pod is orphaned", "pod", pod.Name, "replicaSet", replicaSetOwner.Name)
		return true
	}

	if err != nil {
		log.Error(err, "Error checking ReplicaSet existence", "replicaSet", replicaSetOwner.Name)
		return false
	}

	// ReplicaSet exists, but check if it's being deleted or has 0 replicas
	if replicaSet.DeletionTimestamp != nil {
		log.Info("ReplicaSet is being deleted, pod is orphaned", "pod", pod.Name, "replicaSet", replicaSetOwner.Name)
		return true
	}

	// Check if ReplicaSet has 0 replicas desired (scaled down)
	if replicaSet.Spec.Replicas != nil && *replicaSet.Spec.Replicas == 0 {
		log.Info("ReplicaSet has 0 replicas, pod is orphaned", "pod", pod.Name, "replicaSet", replicaSetOwner.Name)
		return true
	}

	return false
}

// SetupWithManager sets up the controller with the Manager
func (r *PodCleanupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		WithEventFilter(predicate.NewPredicateFuncs(func(object client.Object) bool {
			pod, ok := object.(*corev1.Pod)
			if !ok {
				return false
			}

			// Watch all pods that use SharedVolumes (have our annotations)
			if pod.Annotations != nil {
				if sharedVolumeList, exists := pod.Annotations[SharedVolumeAnnotationKey]; exists && sharedVolumeList != "" {
					return true
				}
			}

			// Also watch pods that already have our finalizer
			if r.hasFinalizer(pod, SharedVolumePodFinalizer) {
				return true
			}

			// Watch all pods that might be orphaned by their ReplicaSet
			for _, owner := range pod.OwnerReferences {
				if owner.Kind == "ReplicaSet" && owner.APIVersion == "apps/v1" {
					return true
				}
			}

			return false
		})).
		Named("pod-cleanup").
		Complete(r)
}
