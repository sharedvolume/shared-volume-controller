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

package resources

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/types"
	"github.com/sharedvolume/shared-volume-controller/internal/controller/utils"
)

// PersistentVolumeClaimManager manages PersistentVolumeClaim resources
type PersistentVolumeClaimManager struct {
	Client  client.Client
	NameGen *utils.NameGenerator
}

// NewPersistentVolumeClaimManager creates a new PersistentVolumeClaimManager instance
func NewPersistentVolumeClaimManager(client client.Client) *PersistentVolumeClaimManager {
	return &PersistentVolumeClaimManager{
		Client:  client,
		NameGen: utils.NewNameGenerator(),
	}
}

// Reconcile ensures the PersistentVolumeClaim exists and is in the desired state
func (p *PersistentVolumeClaimManager) Reconcile(ctx context.Context, volumeObj types.VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Generate PVC name
	pvcName := p.GeneratePVCName(spec.ReferenceValue)
	pvName := fmt.Sprintf("pv-%s", spec.ReferenceValue)

	// Check if PVC already exists
	existingPVC := &corev1.PersistentVolumeClaim{}
	err := p.Client.Get(ctx, client.ObjectKey{
		Name:      pvcName,
		Namespace: namespace,
	}, existingPVC)

	if err == nil {
		// PVC already exists, update status if needed
		if volumeObj.GetPersistentVolumeClaimName() != pvcName {
			log.Info("PVC already exists", "name", pvcName)
		}
		return nil
	}

	if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to check if PVC exists")
		return err
	}

	// Create the PVC
	return p.CreatePersistentVolumeClaim(ctx, volumeObj, namespace, pvcName, pvName)
}

// IsReady checks if the PersistentVolumeClaim is ready
func (p *PersistentVolumeClaimManager) IsReady(ctx context.Context, volumeObj types.VolumeObject, namespace string) (bool, error) {
	spec := volumeObj.GetVolumeSpec()
	pvcName := p.GeneratePVCName(spec.ReferenceValue)

	pvc := &corev1.PersistentVolumeClaim{}
	err := p.Client.Get(ctx, client.ObjectKey{
		Name:      pvcName,
		Namespace: namespace,
	}, pvc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Check if PVC is bound
	return pvc.Status.Phase == corev1.ClaimBound, nil
}

// CreatePersistentVolumeClaim creates a new PersistentVolumeClaim
func (p *PersistentVolumeClaimManager) CreatePersistentVolumeClaim(ctx context.Context, volumeObj types.VolumeObject, namespace, pvcName, pvName string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Get storage capacity as a resource quantity
	storageQuantity, err := resource.ParseQuantity(spec.Storage.Capacity)
	if err != nil {
		log.Error(err, "Failed to parse storage capacity", "capacity", spec.Storage.Capacity)
		return err
	}

	// Define the PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "shared-volume-controller",
				"shared-volume.io/reference":   spec.ReferenceValue,
				"shared-volume.io/owner":       fmt.Sprintf("%s.%s", volumeObj.GetName(), namespace),
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: volumeObj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
					Kind:       volumeObj.GetObjectKind().GroupVersionKind().Kind,
					Name:       volumeObj.GetName(),
					UID:        volumeObj.GetUID(),
					Controller: pointer.Bool(false),
				},
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageQuantity,
				},
			},
			VolumeName:       pvName,
			StorageClassName: pointer.String(""), // Set to empty string explicitly
		},
	}

	// Create the PVC
	if err := p.Client.Create(ctx, pvc); err != nil {
		log.Error(err, "Failed to create PersistentVolumeClaim", "name", pvcName)
		return err
	}

	log.Info("Created PersistentVolumeClaim", "name", pvcName)
	return nil
}

// GeneratePVCName generates a PVC name based on reference value
func (p *PersistentVolumeClaimManager) GeneratePVCName(referenceValue string) string {
	return referenceValue
}

// DeletePersistentVolumeClaim deletes a PersistentVolumeClaim
func (p *PersistentVolumeClaimManager) DeletePersistentVolumeClaim(ctx context.Context, pvcName, namespace string) error {
	log := logf.FromContext(ctx)

	pvc := &corev1.PersistentVolumeClaim{}
	err := p.Client.Get(ctx, client.ObjectKey{
		Name:      pvcName,
		Namespace: namespace,
	}, pvc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PVC already deleted", "name", pvcName, "namespace", namespace)
			return nil
		}
		return err
	}

	if err := p.Client.Delete(ctx, pvc); err != nil {
		log.Error(err, "Failed to delete PersistentVolumeClaim", "name", pvcName)
		return err
	}

	log.Info("Deleted PersistentVolumeClaim", "name", pvcName, "namespace", namespace)
	return nil
}

// GetPersistentVolumeClaim retrieves a PersistentVolumeClaim by name and namespace
func (p *PersistentVolumeClaimManager) GetPersistentVolumeClaim(ctx context.Context, pvcName, namespace string) (*corev1.PersistentVolumeClaim, error) {
	pvc := &corev1.PersistentVolumeClaim{}
	err := p.Client.Get(ctx, client.ObjectKey{
		Name:      pvcName,
		Namespace: namespace,
	}, pvc)
	return pvc, err
}

// UpdatePersistentVolumeClaimStatus updates the PVC status in the volume object
func (p *PersistentVolumeClaimManager) UpdatePersistentVolumeClaimStatus(volumeObj types.VolumeObject, pvcName string) {
	if volumeObj.GetPersistentVolumeClaimName() != pvcName {
		volumeObj.SetPersistentVolumeClaimName(pvcName)
	}
}

// IsPVCBound checks if a PVC is bound to a PV
func (p *PersistentVolumeClaimManager) IsPVCBound(ctx context.Context, pvcName, namespace string) (bool, error) {
	pvc, err := p.GetPersistentVolumeClaim(ctx, pvcName, namespace)
	if err != nil {
		return false, err
	}
	return pvc.Status.Phase == corev1.ClaimBound, nil
}
