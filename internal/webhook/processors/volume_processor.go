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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ktypes "k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
	"github.com/sharedvolume/shared-volume-controller/internal/webhook/types"
	"github.com/sharedvolume/shared-volume-controller/internal/webhook/utils"
)

// VolumeProcessor handles shared volume processing operations
type VolumeProcessor struct {
	client client.Client
	config *types.WebhookConfig
}

// NewVolumeProcessor creates a new volume processor
func NewVolumeProcessor(client client.Client, config *types.WebhookConfig) *VolumeProcessor {
	return &VolumeProcessor{
		client: client,
		config: config,
	}
}

// ProcessSharedVolume handles the creation of PV, PVC, and volume mounting for a SharedVolume or ClusterSharedVolume
func (p *VolumeProcessor) ProcessSharedVolume(ctx context.Context, pod *corev1.Pod, svRef types.VolumeReference) error {
	logger := log.FromContext(ctx).WithName("volume-processor")

	var sharedVolumeSpec *svv1alpha1.SharedVolumeSpec
	var referenceValue string

	if svRef.IsCluster {
		// Fetch the ClusterSharedVolume resource
		clusterSharedVolume := &svv1alpha1.ClusterSharedVolume{}
		err := p.client.Get(ctx, ktypes.NamespacedName{
			Name: svRef.Name,
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
		err := p.client.Get(ctx, ktypes.NamespacedName{
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
		resourceNamespace = types.ClusterSharedVolumeOperationNamespace
	} else {
		// SharedVolume uses the pod's namespace
		resourceNamespace = pod.Namespace
	}

	// Create PV if it doesn't exist
	if err := p.ensurePersistentVolume(ctx, sharedVolumeSpec, referenceValue, pod.Namespace, svRef.Name, resourceNamespace); err != nil {
		return fmt.Errorf("failed to ensure PersistentVolume: %w", err)
	}

	// Create PVC in the pod's namespace
	pvcNamespace := pod.Namespace
	if err := p.ensurePersistentVolumeClaim(ctx, sharedVolumeSpec, referenceValue, pvcNamespace); err != nil {
		return fmt.Errorf("failed to ensure PersistentVolumeClaim: %w", err)
	}

	// Add volume mount to pod
	p.addVolumeToPod(pod, sharedVolumeSpec, referenceValue, pvcNamespace)

	logger.Info("Successfully processed volume", "volume", svRef.Name, "pod", pod.Name)
	return nil
}

// CleanupVolumeIfUnused cleans up PV/PVC for a shared volume if no other pods are using it
func (p *VolumeProcessor) CleanupVolumeIfUnused(ctx context.Context, svRef types.VolumeReference, deletingPod *corev1.Pod) error {
	logger := log.FromContext(ctx).WithName("volume-cleanup")

	// Get the referenceValue from the SharedVolume/ClusterSharedVolume resource
	var referenceValue string
	if svRef.IsCluster {
		clusterSharedVolume := &svv1alpha1.ClusterSharedVolume{}
		if err := p.client.Get(ctx, ktypes.NamespacedName{Name: svRef.Name}, clusterSharedVolume); err != nil {
			return fmt.Errorf("failed to get ClusterSharedVolume %s for cleanup: %w", svRef.Name, err)
		}
		referenceValue = clusterSharedVolume.Spec.ReferenceValue
	} else {
		sharedVolume := &svv1alpha1.SharedVolume{}
		if err := p.client.Get(ctx, ktypes.NamespacedName{Name: svRef.Name, Namespace: svRef.Namespace}, sharedVolume); err != nil {
			return fmt.Errorf("failed to get SharedVolume %s/%s for cleanup: %w", svRef.Namespace, svRef.Name, err)
		}
		referenceValue = sharedVolume.Spec.ReferenceValue
	}

	pvcName := utils.GeneratePVCName(referenceValue, deletingPod.Namespace)
	pvName := utils.GeneratePVName(referenceValue, deletingPod.Namespace)

	// Check if any other pods are still using this specific SharedVolume/ClusterSharedVolume
	stillInUse, err := p.isVolumeStillInUseByOtherPods(ctx, svRef, deletingPod)
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
	if err := p.deletePVC(ctx, pvcName, deletingPod.Namespace); err != nil {
		return fmt.Errorf("failed to delete PVC: %w", err)
	}

	// Delete PV
	if err := p.deletePV(ctx, pvName); err != nil {
		return fmt.Errorf("failed to delete PV: %w", err)
	}

	logger.Info("Successfully cleaned up shared volume resources", "volume", svRef.Name, "pvc", pvcName, "pv", pvName)
	return nil
}

// ensurePersistentVolume creates a PV if it doesn't already exist
func (p *VolumeProcessor) ensurePersistentVolume(ctx context.Context, spec *svv1alpha1.SharedVolumeSpec, referenceValue, pvNamespace, volumeName, volumeNamespace string) error {
	pvName := utils.GeneratePVName(referenceValue, pvNamespace)

	// Check if PV already exists
	existingPV := &corev1.PersistentVolume{}
	err := p.client.Get(ctx, ktypes.NamespacedName{Name: pvName}, existingPV)
	if err == nil {
		return nil // PV already exists
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

	if err := p.client.Create(ctx, pv); err != nil {
		return fmt.Errorf("failed to create PersistentVolume: %w", err)
	}

	return nil
}

// ensurePersistentVolumeClaim creates a PVC if it doesn't already exist
func (p *VolumeProcessor) ensurePersistentVolumeClaim(ctx context.Context, spec *svv1alpha1.SharedVolumeSpec, referenceValue, resourceNamespace string) error {
	pvcName := utils.GeneratePVCName(referenceValue, resourceNamespace)
	pvName := utils.GeneratePVName(referenceValue, resourceNamespace)

	// Check if PVC already exists
	existingPVC := &corev1.PersistentVolumeClaim{}
	err := p.client.Get(ctx, ktypes.NamespacedName{
		Name:      pvcName,
		Namespace: resourceNamespace,
	}, existingPVC)
	if err == nil {
		return nil // PVC already exists
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

	if err := p.client.Create(ctx, pvc); err != nil {
		return fmt.Errorf("failed to create PersistentVolumeClaim: %w", err)
	}

	return nil
}

// addVolumeToPod adds volume and mount to pod
func (p *VolumeProcessor) addVolumeToPod(pod *corev1.Pod, spec *svv1alpha1.SharedVolumeSpec, referenceValue, resourceNamespace string) {
	volumeName := referenceValue
	pvcName := utils.GeneratePVCName(referenceValue, resourceNamespace)

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
				Name:      volumeName,
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
			Name: volumeName,
			VolumeSource: corev1.VolumeSource{
				PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: pvcName,
				},
			},
		})
	}
}

// isVolumeStillInUseByOtherPods checks if a specific SharedVolume or ClusterSharedVolume is still being used by other pods
func (p *VolumeProcessor) isVolumeStillInUseByOtherPods(ctx context.Context, svRef types.VolumeReference, deletingPod *corev1.Pod) (bool, error) {
	// List all pods in all namespaces to check for SharedVolume/ClusterSharedVolume usage
	podList := &corev1.PodList{}
	if err := p.client.List(ctx, podList); err != nil {
		return false, fmt.Errorf("failed to list pods: %w", err)
	}

	annotationProcessor := NewAnnotationProcessor(p.config)

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
		podSharedVolumes := annotationProcessor.ExtractVolumeReferences(&pod)
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
func (p *VolumeProcessor) deletePVC(ctx context.Context, pvcName, namespace string) error {
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
		},
	}

	err := p.client.Delete(ctx, pvc)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete PVC %s/%s: %w", namespace, pvcName, err)
	}

	return nil
}

// deletePV deletes a PV with proper error handling
func (p *VolumeProcessor) deletePV(ctx context.Context, pvName string) error {
	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
		},
	}

	err := p.client.Delete(ctx, pv)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete PV %s: %w", pvName, err)
	}

	return nil
}
