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
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/sharedvolume/shared-volume-controller/internal/webhook/types"
)

// ExtractVolumeAnnotations extracts SharedVolume and ClusterSharedVolume references from pod annotations
func ExtractVolumeAnnotations(pod *corev1.Pod) []types.VolumeRef {
	var volumes []types.VolumeRef

	if pod.Annotations == nil {
		return volumes
	}

	// Look for SharedVolume annotation: "sharedvolume.sv": "sv1,sv2,sv3"
	if sharedVolumeList, exists := pod.Annotations[types.SharedVolumeAnnotationKey]; exists && sharedVolumeList != "" {
		svNames := strings.Split(sharedVolumeList, ",")
		for _, svName := range svNames {
			svName = strings.TrimSpace(svName)
			if svName != "" {
				volumes = append(volumes, types.VolumeRef{
					Name:      svName,
					Namespace: pod.Namespace, // SharedVolumes are in the same namespace as the pod
					IsCluster: false,
				})
			}
		}
	}

	// Look for ClusterSharedVolume annotation: "sharedvolume.csv": "csv1,csv2,csv3"
	if clusterVolumeList, exists := pod.Annotations[types.ClusterSharedVolumeAnnotationKey]; exists && clusterVolumeList != "" {
		csvNames := strings.Split(clusterVolumeList, ",")
		for _, csvName := range csvNames {
			csvName = strings.TrimSpace(csvName)
			if csvName != "" {
				volumes = append(volumes, types.VolumeRef{
					Name:      csvName,
					Namespace: types.ClusterSharedVolumeOperationNamespace, // ClusterSharedVolumes use operation namespace
					IsCluster: true,
				})
			}
		}
	}

	return volumes
}

// AddVolumeAnnotations adds volume annotations to the pod
func AddVolumeAnnotations(pod *corev1.Pod, volumes []types.VolumeRef) {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}

	var sharedVolumes []string
	var clusterSharedVolumes []string

	for _, volume := range volumes {
		if volume.IsCluster {
			clusterSharedVolumes = append(clusterSharedVolumes, volume.Name)
		} else {
			sharedVolumes = append(sharedVolumes, volume.Name)
		}
	}

	if len(sharedVolumes) > 0 {
		pod.Annotations[types.SharedVolumeAnnotationKey] = strings.Join(sharedVolumes, ",")
	}

	if len(clusterSharedVolumes) > 0 {
		pod.Annotations[types.ClusterSharedVolumeAnnotationKey] = strings.Join(clusterSharedVolumes, ",")
	}
}

// HasVolumeAnnotations checks if the pod has any volume annotations
func HasVolumeAnnotations(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}

	return pod.Annotations[types.SharedVolumeAnnotationKey] != "" ||
		pod.Annotations[types.ClusterSharedVolumeAnnotationKey] != ""
}
