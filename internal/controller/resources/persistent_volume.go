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
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/types"
	"github.com/sharedvolume/shared-volume-controller/internal/controller/utils"
)

// PersistentVolumeManager manages PersistentVolume resources
type PersistentVolumeManager struct {
	Client  client.Client
	NameGen *utils.NameGenerator
}

// NewPersistentVolumeManager creates a new PersistentVolumeManager instance
func NewPersistentVolumeManager(client client.Client) *PersistentVolumeManager {
	return &PersistentVolumeManager{
		Client:  client,
		NameGen: utils.NewNameGenerator(),
	}
}

// Reconcile ensures the PersistentVolume exists and is in the desired state
func (p *PersistentVolumeManager) Reconcile(ctx context.Context, volumeObj types.VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Generate PV name
	pvName := p.GeneratePVName(spec.ReferenceValue)

	// Check if PV already exists
	existingPV := &corev1.PersistentVolume{}
	err := p.Client.Get(ctx, client.ObjectKey{Name: pvName}, existingPV)
	if err == nil {
		// PV already exists, update status if needed
		if volumeObj.GetPersistentVolumeName() != pvName {
			log.Info("PV already exists", "name", pvName)
		}
		return nil
	}

	if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to check if PV exists")
		return err
	}

	// Create the PV
	return p.CreatePersistentVolume(ctx, volumeObj, namespace, pvName)
}

// IsReady checks if the PersistentVolume is ready
func (p *PersistentVolumeManager) IsReady(ctx context.Context, volumeObj types.VolumeObject, namespace string) (bool, error) {
	spec := volumeObj.GetVolumeSpec()
	pvName := p.GeneratePVName(spec.ReferenceValue)

	pv := &corev1.PersistentVolume{}
	err := p.Client.Get(ctx, client.ObjectKey{Name: pvName}, pv)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Check if PV is available or bound
	return pv.Status.Phase == corev1.VolumeAvailable || pv.Status.Phase == corev1.VolumeBound, nil
}

// CreatePersistentVolume creates a new PersistentVolume
func (p *PersistentVolumeManager) CreatePersistentVolume(ctx context.Context, volumeObj types.VolumeObject, namespace, pvName string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Get storage capacity as a resource quantity
	storageQuantity, err := resource.ParseQuantity(spec.Storage.Capacity)
	if err != nil {
		log.Error(err, "Failed to parse storage capacity", "capacity", spec.Storage.Capacity)
		return err
	}

	// Use the path from the spec for the share
	nfsSharePath := spec.NfsServer.Path
	if nfsSharePath == "" {
		nfsSharePath = "/"
	}

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "shared-volume-controller",
				"shared-volume.io/reference":   spec.ReferenceValue,
				"shared-volume.io/owner":       fmt.Sprintf("%s.%s", volumeObj.GetName(), namespace),
			},
			Annotations: map[string]string{
				"pv.kubernetes.io/provisioned-by": "nfs.csi.k8s.io",
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: storageQuantity,
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			MountOptions: []string{
				"nfsvers=4.1",
			},
			StorageClassName: "",
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver: "nfs.csi.k8s.io",
					VolumeHandle: fmt.Sprintf("%s%s##",
						spec.NfsServer.URL,
						nfsSharePath),
					VolumeAttributes: map[string]string{
						"server": spec.NfsServer.URL,
						"share":  nfsSharePath,
					},
				},
			},
		},
	}

	// Create the PV
	if err := p.Client.Create(ctx, pv); err != nil {
		log.Error(err, "Failed to create PersistentVolume", "name", pvName)
		return err
	}

	log.Info("Created PersistentVolume", "name", pvName)
	return nil
}

// GeneratePVName generates a PV name based on reference value
func (p *PersistentVolumeManager) GeneratePVName(referenceValue string) string {
	return fmt.Sprintf("pv-%s", referenceValue)
}

// DeletePersistentVolume deletes a PersistentVolume
func (p *PersistentVolumeManager) DeletePersistentVolume(ctx context.Context, pvName string) error {
	log := logf.FromContext(ctx)

	pv := &corev1.PersistentVolume{}
	err := p.Client.Get(ctx, client.ObjectKey{Name: pvName}, pv)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PV already deleted", "name", pvName)
			return nil
		}
		return err
	}

	if err := p.Client.Delete(ctx, pv); err != nil {
		log.Error(err, "Failed to delete PersistentVolume", "name", pvName)
		return err
	}

	log.Info("Deleted PersistentVolume", "name", pvName)
	return nil
}

// GetPersistentVolume retrieves a PersistentVolume by name
func (p *PersistentVolumeManager) GetPersistentVolume(ctx context.Context, pvName string) (*corev1.PersistentVolume, error) {
	pv := &corev1.PersistentVolume{}
	err := p.Client.Get(ctx, client.ObjectKey{Name: pvName}, pv)
	return pv, err
}

// UpdatePersistentVolumeStatus updates the PV status in the volume object
func (p *PersistentVolumeManager) UpdatePersistentVolumeStatus(volumeObj types.VolumeObject, pvName string) {
	if volumeObj.GetPersistentVolumeName() != pvName {
		volumeObj.SetPersistentVolumeName(pvName)
	}
}
