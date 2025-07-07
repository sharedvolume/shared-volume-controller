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

package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

const (
	// pvPvcNameTemplate is the template for PV and PVC names
	pvPvcNameTemplate = "%s-%s"
	// SharedVolumePodFinalizer is the finalizer added to pods using SharedVolumes
	SharedVolumePodFinalizer = "sharedvolume.sv/pod-cleanup"
	// SharedVolumeAnnotationKey for SharedVolume annotation
	SharedVolumeAnnotationKey = "sharedvolume.sv"
	// ClusterSharedVolumeAnnotationKey for ClusterSharedVolume annotation
	ClusterSharedVolumeAnnotationKey = "sharedvolume.csv"
	// ClusterSharedVolumeOperationNamespace is the namespace used for ClusterSharedVolume operations
	ClusterSharedVolumeOperationNamespace = "shared-volume-controller-operation"
)

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod.kb.io,admissionReviewVersions=v1

// PodAnnotator annotates Pods
type PodAnnotator struct {
	Client client.Client
}

// Handle processes the admission request
func (a *PodAnnotator) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := log.FromContext(ctx).WithName("pod-webhook")

	pod := &corev1.Pod{}
	if err := json.Unmarshal(req.Object.Raw, pod); err != nil {
		logger.Error(err, "failed to unmarshal pod")
		return admission.Errored(http.StatusBadRequest, err)
	}

	logger.Info("Processing pod", "name", pod.Name, "namespace", pod.Namespace, "operation", req.Operation)

	// Handle pod deletion - clean up PV/PVC if no other pods are using them
	if pod.DeletionTimestamp != nil {
		logger.Info("Pod is being deleted, checking for cleanup", "name", pod.Name, "namespace", pod.Namespace)

		// Only process if pod has our finalizer
		if a.hasSharedVolumeFinalizer(pod) {
			if err := a.handlePodDeletion(ctx, pod); err != nil {
				logger.Error(err, "failed to handle pod deletion cleanup", "name", pod.Name, "namespace", pod.Namespace)
				return admission.Errored(http.StatusInternalServerError, err)
			}
		}

		return admission.Allowed("Pod deletion processed")
	}

	// Skip processing for update operations if the pod already has our finalizer and volumes
	if req.Operation == "UPDATE" && a.hasSharedVolumeFinalizer(pod) && len(pod.Spec.Volumes) > 0 {
		logger.Info("Pod already processed (has finalizer and volumes), skipping", "name", pod.Name, "namespace", pod.Namespace)
		return admission.Allowed("Pod already processed")
	}

	// Check for sharedvolume.sv and sharedvolume.csv annotations with comma-separated volume names
	sharedVolumes := a.extractSharedVolumeAnnotations(pod)
	if len(sharedVolumes) == 0 {
		// No shared volume annotations found, return without modifications
		return admission.Allowed("No shared volume annotations found")
	}

	// Process each shared volume
	for _, svRef := range sharedVolumes {
		if err := a.processSharedVolume(ctx, pod, svRef); err != nil {
			logger.Error(err, "failed to process shared volume", "sharedVolume", svRef.Name, "namespace", svRef.Namespace)
			return admission.Errored(http.StatusInternalServerError, err)
		}
	}

	// Add finalizer to ensure cleanup when pod is deleted
	a.addSharedVolumeFinalizer(pod)

	// Marshal the modified pod
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		logger.Error(err, "failed to marshal pod")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

// SharedVolumeRef represents a SharedVolume reference with namespace and name
type SharedVolumeRef struct {
	Namespace string
	Name      string
	IsCluster bool // true for ClusterSharedVolume, false for SharedVolume
}

// extractSharedVolumeAnnotations extracts SharedVolume references from annotations
func (a *PodAnnotator) extractSharedVolumeAnnotations(pod *corev1.Pod) []SharedVolumeRef {
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
					IsCluster: false, // This is a regular SharedVolume
				})
			}
		}
	}

	// Look for ClusterSharedVolume annotation: "sharedvolume.csv": "csv1,csv2,csv3"
	if clusterSharedVolumeList, exists := pod.Annotations[ClusterSharedVolumeAnnotationKey]; exists && clusterSharedVolumeList != "" {
		// Split by comma and trim spaces
		csvNames := strings.Split(clusterSharedVolumeList, ",")
		for _, csvName := range csvNames {
			csvName = strings.TrimSpace(csvName)
			if csvName != "" {
				// ClusterSharedVolumes are cluster-scoped, so no namespace needed
				sharedVolumes = append(sharedVolumes, SharedVolumeRef{
					Namespace: "", // ClusterSharedVolumes don't have a namespace
					Name:      csvName,
					IsCluster: true, // This is a ClusterSharedVolume
				})
			}
		}
	}

	return sharedVolumes
}

// processSharedVolume handles the creation of PV, PVC, and volume mounting for a SharedVolume or ClusterSharedVolume
func (a *PodAnnotator) processSharedVolume(ctx context.Context, pod *corev1.Pod, svRef SharedVolumeRef) error {
	logger := log.FromContext(ctx).WithName("shared-volume-processor")

	var sharedVolumeSpec *svv1alpha1.SharedVolumeSpec
	var referenceValue string

	if svRef.IsCluster {
		// Fetch the ClusterSharedVolume resource
		clusterSharedVolume := &svv1alpha1.ClusterSharedVolume{}
		err := a.Client.Get(ctx, types.NamespacedName{
			Name: svRef.Name, // ClusterSharedVolume has no namespace
		}, clusterSharedVolume)
		if err != nil {
			return fmt.Errorf("failed to get ClusterSharedVolume %s: %w", svRef.Name, err)
		}

		logger.Info("Processing ClusterSharedVolume", "clusterSharedVolume", svRef.Name, "referenceValue", clusterSharedVolume.Spec.ReferenceValue, "pod", pod.Name)
		// Convert ClusterSharedVolumeSpec to SharedVolumeSpec (they have the same structure)
		sharedVolumeSpec = &svv1alpha1.SharedVolumeSpec{
			VolumeSpecBase: clusterSharedVolume.Spec.VolumeSpecBase,
		}
		referenceValue = clusterSharedVolume.Spec.ReferenceValue
	} else {
		// Fetch the SharedVolume resource
		sharedVolume := &svv1alpha1.SharedVolume{}
		err := a.Client.Get(ctx, types.NamespacedName{
			Name:      svRef.Name,
			Namespace: svRef.Namespace,
		}, sharedVolume)
		if err != nil {
			return fmt.Errorf("failed to get SharedVolume %s/%s: %w", svRef.Namespace, svRef.Name, err)
		}

		logger.Info("Processing SharedVolume", "sharedVolume", svRef.Name, "referenceValue", sharedVolume.Spec.ReferenceValue, "namespace", svRef.Namespace, "pod", pod.Name)
		sharedVolumeSpec = &sharedVolume.Spec
		referenceValue = sharedVolume.Spec.ReferenceValue
	}

	// Determine the resource namespace based on volume type
	var resourceNamespace string
	if svRef.IsCluster {
		// ClusterSharedVolume uses a static namespace for resource creation
		resourceNamespace = ClusterSharedVolumeOperationNamespace
	} else {
		// SharedVolume uses the pod's namespace
		resourceNamespace = pod.Namespace
	}

	// Create PV if it doesn't exist - use pod namespace for consistent naming with PVC
	if err := a.ensurePersistentVolumeGeneric(ctx, sharedVolumeSpec, referenceValue, pod.Namespace, svRef.Name, resourceNamespace); err != nil {
		return fmt.Errorf("failed to ensure PersistentVolume: %w", err)
	}

	// For webhook, always create PVC in the pod's namespace
	// The controller will manage the main PVC in the operation namespace
	pvcNamespace := pod.Namespace
	if err := a.ensurePersistentVolumeClaimGeneric(ctx, sharedVolumeSpec, referenceValue, pvcNamespace); err != nil {
		return fmt.Errorf("failed to ensure PersistentVolumeClaim: %w", err)
	}

	// Add volume mount to pod (use the PVC in the pod's namespace)
	a.addVolumeToPodGeneric(pod, sharedVolumeSpec, referenceValue, pvcNamespace)

	logger.Info("Successfully processed volume", "volume", svRef.Name, "pod", pod.Name)
	return nil
}

// ensurePersistentVolume creates a PV if it doesn't already exist
func (a *PodAnnotator) ensurePersistentVolume(ctx context.Context, sv *svv1alpha1.SharedVolume, podNamespace string) error {
	pvName := fmt.Sprintf(pvPvcNameTemplate, sv.Spec.ReferenceValue, podNamespace)

	// Check if PV already exists
	existingPV := &corev1.PersistentVolume{}
	err := a.Client.Get(ctx, types.NamespacedName{Name: pvName}, existingPV)
	if err == nil {
		// PV already exists
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check if PV exists: %w", err)
	}

	// Create new PV
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(sv.Spec.Storage.Capacity),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			MountOptions: []string{
				"nfsvers=4.1",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       "nfs.csi.k8s.io",
					VolumeHandle: fmt.Sprintf("%s%s/%s-%s##", sv.Status.NfsServerAddress, sv.Spec.NfsServer.Path, sv.Name, sv.Namespace),
					VolumeAttributes: map[string]string{
						"server": sv.Status.NfsServerAddress,
						"share":  fmt.Sprintf("%s/%s-%s", sv.Spec.NfsServer.Path, sv.Name, sv.Namespace),
					},
				},
			},
		},
	}

	if err := a.Client.Create(ctx, pv); err != nil {
		return fmt.Errorf("failed to create PersistentVolume: %w", err)
	}

	return nil
}

// ensurePersistentVolumeClaim creates a PVC if it doesn't already exist
func (a *PodAnnotator) ensurePersistentVolumeClaim(ctx context.Context, sv *svv1alpha1.SharedVolume, podNamespace string) error {
	pvcName := fmt.Sprintf(pvPvcNameTemplate, sv.Spec.ReferenceValue, podNamespace)
	pvName := fmt.Sprintf(pvPvcNameTemplate, sv.Spec.ReferenceValue, podNamespace)

	// Check if PVC already exists
	existingPVC := &corev1.PersistentVolumeClaim{}
	err := a.Client.Get(ctx, types.NamespacedName{
		Name:      pvcName,
		Namespace: podNamespace,
	}, existingPVC)
	if err == nil {
		// PVC already exists
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check if PVC exists: %w", err)
	}

	// Create new PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: podNamespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			StorageClassName: pointer.String(""), // Explicitly set to empty string to disable default storage class
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(sv.Spec.Storage.Capacity),
				},
			},
			VolumeName: pvName, // Bind to the specific PV
		},
	}

	if err := a.Client.Create(ctx, pvc); err != nil {
		return fmt.Errorf("failed to create PersistentVolumeClaim: %w", err)
	}

	return nil
}

// ensurePersistentVolumeGeneric creates a PV if it doesn't already exist, using the generic interface
func (a *PodAnnotator) ensurePersistentVolumeGeneric(ctx context.Context, spec *svv1alpha1.SharedVolumeSpec, referenceValue, pvNamespace, volumeName, volumeNamespace string) error {
	pvName := fmt.Sprintf(pvPvcNameTemplate, referenceValue, pvNamespace)

	// Check if PV already exists
	existingPV := &corev1.PersistentVolume{}
	err := a.Client.Get(ctx, types.NamespacedName{Name: pvName}, existingPV)
	if err == nil {
		// PV already exists
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check if PV exists: %w", err)
	}

	// Get storage spec
	storage := spec.Storage
	if storage == nil {
		return fmt.Errorf("storage spec is required")
	}

	// Get NFS server spec
	nfsServer := spec.NfsServer
	if nfsServer == nil {
		return fmt.Errorf("nfsServer spec is required")
	}

	// Build share path - match controller's folder structure: {basePath}/{volumeName}-{namespace}
	// For csv1 ClusterSharedVolume, this will create: /csv1-shared-volume-controller-operation
	// For sv1 SharedVolume in myns namespace, this will create: /sv1-myns
	sharePath := fmt.Sprintf("%s/%s-%s", nfsServer.Path, volumeName, volumeNamespace)

	// Create new PV
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(storage.Capacity),
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			MountOptions: []string{
				"nfsvers=4.1",
			},
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver:       "nfs.csi.k8s.io",
					VolumeHandle: fmt.Sprintf("%s%s##", nfsServer.URL, sharePath),
					VolumeAttributes: map[string]string{
						"server": nfsServer.URL,
						"share":  sharePath,
					},
				},
			},
		},
	}

	if err := a.Client.Create(ctx, pv); err != nil {
		return fmt.Errorf("failed to create PersistentVolume: %w", err)
	}

	return nil
}

// ensurePersistentVolumeClaimGeneric creates a PVC if it doesn't already exist, using the generic interface
func (a *PodAnnotator) ensurePersistentVolumeClaimGeneric(ctx context.Context, spec *svv1alpha1.SharedVolumeSpec, referenceValue, resourceNamespace string) error {
	pvcName := fmt.Sprintf(pvPvcNameTemplate, referenceValue, resourceNamespace)
	pvName := fmt.Sprintf(pvPvcNameTemplate, referenceValue, resourceNamespace)

	// Check if PVC already exists
	existingPVC := &corev1.PersistentVolumeClaim{}
	err := a.Client.Get(ctx, types.NamespacedName{
		Name:      pvcName,
		Namespace: resourceNamespace,
	}, existingPVC)
	if err == nil {
		// PVC already exists
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check if PVC exists: %w", err)
	}

	// Get storage spec
	storage := spec.Storage
	if storage == nil {
		return fmt.Errorf("storage spec is required")
	}

	// Create new PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: resourceNamespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			StorageClassName: pointer.String(""), // Explicitly set to empty string to disable default storage class
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storage.Capacity),
				},
			},
			VolumeName: pvName, // Bind to the specific PV
		},
	}

	if err := a.Client.Create(ctx, pvc); err != nil {
		return fmt.Errorf("failed to create PersistentVolumeClaim: %w", err)
	}

	return nil
}

// ensureProxyPersistentVolumeClaim creates a PVC in the pod's namespace that references the same PV
// This is needed for ClusterSharedVolume where the main PVC is in shared-volume-controller-operation
// but the pod needs a PVC in its own namespace to reference
func (a *PodAnnotator) ensureProxyPersistentVolumeClaim(ctx context.Context, spec *svv1alpha1.SharedVolumeSpec, referenceValue, mainNamespace, podNamespace string) error {
	pvName := fmt.Sprintf(pvPvcNameTemplate, referenceValue, mainNamespace)
	pvcName := fmt.Sprintf(pvPvcNameTemplate, referenceValue, mainNamespace) // Use same PVC name for consistency

	// Check if proxy PVC already exists in pod's namespace
	existingPVC := &corev1.PersistentVolumeClaim{}
	err := a.Client.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: podNamespace}, existingPVC)
	if err == nil {
		// PVC already exists
		return nil
	}
	if !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check if proxy PVC exists: %w", err)
	}

	// Get storage spec
	storage := spec.Storage
	if storage == nil {
		return fmt.Errorf("storage spec is required")
	}

	// Create new proxy PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: podNamespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storage.Capacity),
				},
			},
			VolumeName:       pvName,      // Reference the existing PV
			StorageClassName: new(string), // Empty string to prevent dynamic provisioning
		},
	}

	if err := a.Client.Create(ctx, pvc); err != nil {
		return fmt.Errorf("failed to create proxy PersistentVolumeClaim: %w", err)
	}

	return nil
}

// addVolumeToPod adds volume mount and volume to the pod
func (a *PodAnnotator) addVolumeToPod(pod *corev1.Pod, sv *svv1alpha1.SharedVolume, podNamespace string) {
	volumeName := sv.Spec.ReferenceValue                                            // Use ReferenceValue as the volume name
	pvcName := fmt.Sprintf(pvPvcNameTemplate, sv.Spec.ReferenceValue, podNamespace) // Use sv-ReferenceValue-podNamespace for PVC name

	// Determine if volume should be read-only
	readOnly := sv.Spec.Storage.AccessMode == "ReadOnly"

	// Add volume mount to all containers
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].VolumeMounts == nil {
			pod.Spec.Containers[i].VolumeMounts = []corev1.VolumeMount{}
		}

		// Check if volume mount already exists
		volumeMountExists := false
		for _, vm := range pod.Spec.Containers[i].VolumeMounts {
			if vm.Name == volumeName {
				volumeMountExists = true
				break
			}
		}

		if !volumeMountExists {
			pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, corev1.VolumeMount{
				MountPath: sv.Spec.MountPath,
				Name:      volumeName, // Use the same name as the volume
				ReadOnly:  readOnly,
			})
		}
	}

	// Add volume to pod spec
	if pod.Spec.Volumes == nil {
		pod.Spec.Volumes = []corev1.Volume{}
	}

	// Check if volume already exists
	volumeExists := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == volumeName {
			volumeExists = true
			break
		}
	}

	if !volumeExists {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: volumeName, // Use ReferenceValue as volume name
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		})
	}
}

// addVolumeToPodGeneric adds volume and mount to pod, using the generic interface
func (a *PodAnnotator) addVolumeToPodGeneric(pod *corev1.Pod, spec *svv1alpha1.SharedVolumeSpec, referenceValue, resourceNamespace string) {
	volumeName := referenceValue                                                 // Use ReferenceValue as the volume name
	pvcName := fmt.Sprintf(pvPvcNameTemplate, referenceValue, resourceNamespace) // Use sv-ReferenceValue-resourceNamespace for PVC name

	// Get storage spec
	storage := spec.Storage
	var readOnly bool
	if storage != nil {
		readOnly = storage.AccessMode == "ReadOnly"
	}

	// Add volume mount to all containers
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].VolumeMounts == nil {
			pod.Spec.Containers[i].VolumeMounts = []corev1.VolumeMount{}
		}

		// Check if volume mount already exists
		volumeMountExists := false
		for _, vm := range pod.Spec.Containers[i].VolumeMounts {
			if vm.Name == volumeName {
				volumeMountExists = true
				break
			}
		}

		if !volumeMountExists {
			pod.Spec.Containers[i].VolumeMounts = append(pod.Spec.Containers[i].VolumeMounts, corev1.VolumeMount{
				MountPath: spec.MountPath,
				Name:      volumeName, // Use the same name as the volume
				ReadOnly:  readOnly,
			})
		}
	}

	// Add volume to pod spec
	if pod.Spec.Volumes == nil {
		pod.Spec.Volumes = []corev1.Volume{}
	}

	// Check if volume already exists
	volumeExists := false
	for _, v := range pod.Spec.Volumes {
		if v.Name == volumeName {
			volumeExists = true
			break
		}
	}

	if !volumeExists {
		pod.Spec.Volumes = append(pod.Spec.Volumes, corev1.Volume{
			Name: volumeName, // Use ReferenceValue as volume name
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		})
	}
}

// addSharedVolumeFinalizer adds a finalizer to the pod to ensure cleanup when deleted
func (a *PodAnnotator) addSharedVolumeFinalizer(pod *corev1.Pod) {
	// Check if finalizer already exists
	for _, finalizer := range pod.Finalizers {
		if finalizer == SharedVolumePodFinalizer {
			return // Already exists
		}
	}

	// Add the finalizer
	pod.Finalizers = append(pod.Finalizers, SharedVolumePodFinalizer)
}

// hasSharedVolumeFinalizer checks if the pod has the SharedVolume finalizer
func (a *PodAnnotator) hasSharedVolumeFinalizer(pod *corev1.Pod) bool {
	for _, finalizer := range pod.Finalizers {
		if finalizer == SharedVolumePodFinalizer {
			return true
		}
	}
	return false
}

// handlePodDeletion handles cleanup when a pod with shared volumes is being deleted
func (a *PodAnnotator) handlePodDeletion(ctx context.Context, pod *corev1.Pod) error {
	logger := log.FromContext(ctx).WithName("pod-deletion-handler")

	// Extract shared volume references from the pod annotations
	sharedVolumes := a.extractSharedVolumeAnnotations(pod)
	if len(sharedVolumes) == 0 {
		logger.Info("No shared volumes found in pod, removing finalizer", "pod", pod.Name)
		return a.removePodFinalizer(ctx, pod)
	}

	// For each shared volume, check if any other pods are still using it
	for _, svRef := range sharedVolumes {
		if err := a.cleanupSharedVolumeIfUnused(ctx, svRef, pod); err != nil {
			logger.Error(err, "failed to cleanup shared volume", "volume", svRef.Name, "pod", pod.Name)
			return err
		}
	}

	// Remove finalizer from pod after successful cleanup
	return a.removePodFinalizer(ctx, pod)
}

// cleanupSharedVolumeIfUnused cleans up PV/PVC for a shared volume if no other pods are using it
func (a *PodAnnotator) cleanupSharedVolumeIfUnused(ctx context.Context, svRef SharedVolumeRef, deletingPod *corev1.Pod) error {
	logger := log.FromContext(ctx).WithName("shared-volume-cleanup")

	// Get the referenceValue from the SharedVolume/ClusterSharedVolume resource
	var referenceValue string
	if svRef.IsCluster {
		clusterSharedVolume := &svv1alpha1.ClusterSharedVolume{}
		if err := a.Client.Get(ctx, types.NamespacedName{Name: svRef.Name}, clusterSharedVolume); err != nil {
			return fmt.Errorf("failed to get ClusterSharedVolume %s for cleanup: %w", svRef.Name, err)
		}
		referenceValue = clusterSharedVolume.Spec.ReferenceValue
	} else {
		sharedVolume := &svv1alpha1.SharedVolume{}
		if err := a.Client.Get(ctx, types.NamespacedName{Name: svRef.Name, Namespace: svRef.Namespace}, sharedVolume); err != nil {
			return fmt.Errorf("failed to get SharedVolume %s/%s for cleanup: %w", svRef.Namespace, svRef.Name, err)
		}
		referenceValue = sharedVolume.Spec.ReferenceValue
	}

	// For webhook cleanup, we need to clean up PV/PVC created in the pod's namespace
	// (since webhook now creates everything in pod namespace)
	pvcName := fmt.Sprintf(pvPvcNameTemplate, referenceValue, deletingPod.Namespace)
	pvName := fmt.Sprintf(pvPvcNameTemplate, referenceValue, deletingPod.Namespace)

	// Check if any other pods are still using this specific SharedVolume/ClusterSharedVolume
	stillInUse, err := a.isVolumeStillInUseByOtherPods(ctx, svRef, deletingPod)
	if err != nil {
		return fmt.Errorf("failed to check if volume is still in use: %w", err)
	}

	if stillInUse {
		logger.Info("Volume still in use by other pods, skipping cleanup",
			"volume", svRef.Name,
			"isCluster", svRef.IsCluster,
			"pvc", pvcName,
			"namespace", deletingPod.Namespace)
		return nil
	}

	logger.Info("Volume no longer in use, cleaning up",
		"volume", svRef.Name,
		"isCluster", svRef.IsCluster,
		"pvc", pvcName,
		"pv", pvName,
		"namespace", deletingPod.Namespace)

	// Delete PVC first
	if err := a.deletePVC(ctx, pvcName, deletingPod.Namespace); err != nil {
		return fmt.Errorf("failed to delete PVC: %w", err)
	}

	// Delete PV
	if err := a.deletePV(ctx, pvName); err != nil {
		return fmt.Errorf("failed to delete PV: %w", err)
	}

	logger.Info("Successfully cleaned up shared volume resources", "volume", svRef.Name, "pvc", pvcName, "pv", pvName)
	return nil
}

// isPVCStillInUse checks if any other pods (excluding the deleting pod) are still using the specific SharedVolume/ClusterSharedVolume
func (a *PodAnnotator) isPVCStillInUse(ctx context.Context, pvcName, namespace, deletingPodName string) (bool, error) {
	// List all pods in the same namespace where the PVC exists
	podList := &corev1.PodList{}
	if err := a.Client.List(ctx, podList, client.InNamespace(namespace)); err != nil {
		return false, fmt.Errorf("failed to list pods in namespace %s: %w", namespace, err)
	}

	for _, pod := range podList.Items {
		// Skip the pod that's being deleted
		if pod.Name == deletingPodName {
			continue
		}

		// Skip pods that are also being deleted
		if pod.DeletionTimestamp != nil {
			continue
		}

		// Check if this pod uses the PVC directly
		for _, volume := range pod.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.ClaimName == pvcName {
				return true, nil
			}
		}
	}

	return false, nil
}

// isVolumeStillInUseByOtherPods checks if a specific SharedVolume or ClusterSharedVolume is still being used by other pods
func (a *PodAnnotator) isVolumeStillInUseByOtherPods(ctx context.Context, svRef SharedVolumeRef, deletingPod *corev1.Pod) (bool, error) {
	// List all pods in all namespaces to check for SharedVolume/ClusterSharedVolume usage
	podList := &corev1.PodList{}
	if err := a.Client.List(ctx, podList); err != nil {
		return false, fmt.Errorf("failed to list pods: %w", err)
	}

	for _, pod := range podList.Items {
		// Skip the pod that's being deleted
		if pod.Name == deletingPod.Name && pod.Namespace == deletingPod.Namespace {
			continue
		}

		// Skip pods that are also being deleted
		if pod.DeletionTimestamp != nil {
			continue
		}

		// Check if this pod uses the same SharedVolume or ClusterSharedVolume
		podSharedVolumes := a.extractSharedVolumeAnnotations(&pod)
		for _, podSvRef := range podSharedVolumes {
			// Check if this pod references the same volume
			if podSvRef.IsCluster == svRef.IsCluster && podSvRef.Name == svRef.Name {
				// For SharedVolume, also check namespace match
				if !svRef.IsCluster && podSvRef.Namespace != svRef.Namespace {
					continue
				}
				// Found another pod using the same volume
				return true, nil
			}
		}
	}

	return false, nil
}

// deletePVC deletes a PVC with proper error handling
func (a *PodAnnotator) deletePVC(ctx context.Context, pvcName, namespace string) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
	}

	err := a.Client.Delete(ctx, pvc)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete PVC %s/%s: %w", namespace, pvcName, err)
	}

	return nil
}

// deletePV deletes a PV with proper error handling
func (a *PodAnnotator) deletePV(ctx context.Context, pvName string) error {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
	}

	err := a.Client.Delete(ctx, pv)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete PV %s: %w", pvName, err)
	}

	return nil
}

// removePodFinalizer removes the shared volume finalizer from the pod
func (a *PodAnnotator) removePodFinalizer(ctx context.Context, pod *corev1.Pod) error {
	// Remove the finalizer
	var updatedFinalizers []string
	for _, finalizer := range pod.Finalizers {
		if finalizer != SharedVolumePodFinalizer {
			updatedFinalizers = append(updatedFinalizers, finalizer)
		}
	}

	pod.Finalizers = updatedFinalizers

	// Update the pod to remove the finalizer
	if err := a.Client.Update(ctx, pod); err != nil {
		return fmt.Errorf("failed to remove finalizer from pod: %w", err)
	}

	return nil
}
