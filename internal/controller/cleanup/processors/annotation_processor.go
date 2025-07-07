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
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/cleanup/types"
)

// AnnotationProcessor handles annotation extraction and processing for cleanup
type AnnotationProcessor struct {
	config *types.CleanupConfig
}

// NewAnnotationProcessor creates a new annotation processor
func NewAnnotationProcessor(config *types.CleanupConfig) *AnnotationProcessor {
	return &AnnotationProcessor{
		config: config,
	}
}

// ExtractSharedVolumeAnnotations extracts SharedVolume and ClusterSharedVolume references from pod annotations
func (p *AnnotationProcessor) ExtractSharedVolumeAnnotations(pod *corev1.Pod) []types.SharedVolumeRef {
	var sharedVolumes []types.SharedVolumeRef

	if pod.Annotations == nil {
		return sharedVolumes
	}

	// Look for SharedVolume annotation: "sharedvolume.sv": "sv1,sv2,sv3"
	if sharedVolumeList, exists := pod.Annotations[types.SharedVolumeAnnotationKey]; exists && sharedVolumeList != "" {
		// Split by comma and trim spaces
		svNames := strings.Split(sharedVolumeList, ",")
		for _, svName := range svNames {
			svName = strings.TrimSpace(svName)
			if svName != "" {
				// All SharedVolumes are in the same namespace as the pod
				sharedVolumes = append(sharedVolumes, types.SharedVolumeRef{
					Namespace: pod.Namespace,
					Name:      svName,
				})
			}
		}
	}

	// Look for ClusterSharedVolume annotation: "sharedvolume.csv": "csv1,csv2,csv3"
	if clusterSharedVolumeList, exists := pod.Annotations[types.ClusterSharedVolumeAnnotationKey]; exists && clusterSharedVolumeList != "" {
		// Split by comma and trim spaces
		csvNames := strings.Split(clusterSharedVolumeList, ",")
		for _, csvName := range csvNames {
			csvName = strings.TrimSpace(csvName)
			if csvName != "" {
				// ClusterSharedVolumes use the static operation namespace
				sharedVolumes = append(sharedVolumes, types.SharedVolumeRef{
					Namespace: types.ClusterSharedVolumeOperationNamespace, // Static namespace for ClusterSharedVolumes
					Name:      csvName,
				})
			}
		}
	}

	return sharedVolumes
}

// HasSharedVolumeAnnotations checks if a pod has SharedVolume annotations
func (p *AnnotationProcessor) HasSharedVolumeAnnotations(pod *corev1.Pod) bool {
	if pod.Annotations == nil {
		return false
	}

	if sharedVolumeList, exists := pod.Annotations[types.SharedVolumeAnnotationKey]; exists && sharedVolumeList != "" {
		return true
	}

	if clusterSharedVolumeList, exists := pod.Annotations[types.ClusterSharedVolumeAnnotationKey]; exists && clusterSharedVolumeList != "" {
		return true
	}

	return false
}
