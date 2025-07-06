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

	// Skip processing if pod is being deleted
	if pod.DeletionTimestamp != nil {
		logger.Info("Pod is being deleted, skipping webhook processing", "name", pod.Name, "namespace", pod.Namespace)
		return admission.Allowed("Pod is being deleted, skipping processing")
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
				})
			}
		}
	}

	return sharedVolumes
}

// processSharedVolume handles the creation of PV, PVC, and volume mounting for a SharedVolume
func (a *PodAnnotator) processSharedVolume(ctx context.Context, pod *corev1.Pod, svRef SharedVolumeRef) error {
	logger := log.FromContext(ctx).WithName("shared-volume-processor")

	// Fetch the SharedVolume resource
	sharedVolume := &svv1alpha1.SharedVolume{}
	err := a.Client.Get(ctx, types.NamespacedName{
		Name:      svRef.Name,
		Namespace: svRef.Namespace,
	}, sharedVolume)
	if err != nil {
		return fmt.Errorf("failed to get SharedVolume %s/%s: %w", svRef.Namespace, svRef.Name, err)
	}

	logger.Info("Processing SharedVolume", "sharedVolume", svRef.Name, "namespace", svRef.Namespace, "pod", pod.Name)

	// Create PV if it doesn't exist
	if err := a.ensurePersistentVolume(ctx, sharedVolume, pod.Namespace); err != nil {
		return fmt.Errorf("failed to ensure PersistentVolume: %w", err)
	}

	// Create PVC if it doesn't exist
	if err := a.ensurePersistentVolumeClaim(ctx, sharedVolume, pod.Namespace); err != nil {
		return fmt.Errorf("failed to ensure PersistentVolumeClaim: %w", err)
	}

	// Add volume mount to pod
	a.addVolumeToPod(pod, sharedVolume, pod.Namespace)

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
					VolumeHandle: fmt.Sprintf("%s/share##", sv.Status.NfsServerAddress),
					VolumeAttributes: map[string]string{
						"server": sv.Status.NfsServerAddress,
						"share":  a.buildSharePath(sv.Spec.NfsServer.Path, sv.Name, sv.Namespace),
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
func (a *PodAnnotator) ensurePersistentVolumeGeneric(ctx context.Context, spec *svv1alpha1.SharedVolumeSpec, referenceValue, resourceNamespace string) error {
	pvName := fmt.Sprintf(pvPvcNameTemplate, referenceValue, resourceNamespace)

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

	// Build share path
	sharePath := a.buildSharePathGeneric(nfsServer.Path, referenceValue, resourceNamespace)

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
					VolumeHandle: fmt.Sprintf("%s/share##", nfsServer.URL),
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

// buildSharePath constructs the NFS share path, ensuring the base path ends with '/'
func (a *PodAnnotator) buildSharePath(basePath, svName, svNamespace string) string {
	// Ensure basePath ends with '/'
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	return fmt.Sprintf("%s%s-%s", basePath, svName, svNamespace)
}

// buildSharePathGeneric constructs the NFS share path, ensuring the base path ends with '/'
func (a *PodAnnotator) buildSharePathGeneric(basePath, referenceValue, resourceNamespace string) string {
	// Ensure basePath ends with '/'
	if !strings.HasSuffix(basePath, "/") {
		basePath += "/"
	}
	return fmt.Sprintf("%s%s-%s", basePath, referenceValue, resourceNamespace)
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
