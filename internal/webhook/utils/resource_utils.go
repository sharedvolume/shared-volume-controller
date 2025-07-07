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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
	"github.com/sharedvolume/shared-volume-controller/internal/webhook/types"
)

// CreatePVC creates a PersistentVolumeClaim for a volume reference
func CreatePVC(volumeRef types.VolumeRef, namespace, referenceValue, nfsServerURL, nfsPath string) *corev1.PersistentVolumeClaim {
	pvcName := fmt.Sprintf(types.PVPVCNameTemplate, referenceValue, namespace)
	pvName := fmt.Sprintf("pv-%s", pvcName)

	storageQuantity := resource.MustParse(types.DefaultStorageSize)

	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "shared-volume-controller",
				"shared-volume.io/type":        getVolumeType(volumeRef),
				"shared-volume.io/reference":   referenceValue,
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
}

// CreatePV creates a PersistentVolume for a volume reference
func CreatePV(volumeRef types.VolumeRef, namespace, referenceValue, nfsServerURL, nfsPath string) *corev1.PersistentVolume {
	pvcName := fmt.Sprintf(types.PVPVCNameTemplate, referenceValue, namespace)
	pvName := fmt.Sprintf("pv-%s", pvcName)

	storageQuantity := resource.MustParse(types.DefaultStorageSize)

	return &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "shared-volume-controller",
				"shared-volume.io/type":        getVolumeType(volumeRef),
				"shared-volume.io/reference":   referenceValue,
			},
			Annotations: map[string]string{
				"pv.kubernetes.io/provisioned-by": types.NFSCSIDriverName,
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
					Driver:       types.NFSCSIDriverName,
					VolumeHandle: fmt.Sprintf("%s%s##", nfsServerURL, nfsPath),
					VolumeAttributes: map[string]string{
						"server": nfsServerURL,
						"share":  nfsPath,
					},
				},
			},
		},
	}
}

// CreateVolumeMount creates a VolumeMount for a volume reference
func CreateVolumeMount(volumeRef types.VolumeRef, mountPath string) corev1.VolumeMount {
	return corev1.VolumeMount{
		Name:      volumeRef.Name,
		MountPath: mountPath,
	}
}

// CreateVolume creates a Volume for a volume reference
func CreateVolume(volumeRef types.VolumeRef, pvcName string) corev1.Volume {
	return corev1.Volume{
		Name: volumeRef.Name,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvcName,
			},
		},
	}
}

// GetVolumeSpecFromResource extracts volume spec from SharedVolume or ClusterSharedVolume
func GetVolumeSpecFromResource(resource interface{}) (*svv1alpha1.VolumeSpecBase, error) {
	switch v := resource.(type) {
	case *svv1alpha1.SharedVolume:
		return &v.Spec.VolumeSpecBase, nil
	case *svv1alpha1.ClusterSharedVolume:
		return &v.Spec.VolumeSpecBase, nil
	default:
		return nil, fmt.Errorf("unsupported volume resource type: %T", resource)
	}
}

// getVolumeType returns the volume type label value
func getVolumeType(volumeRef types.VolumeRef) string {
	if volumeRef.IsCluster {
		return "cluster-shared-volume"
	}
	return "shared-volume"
}
