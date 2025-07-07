/*
Copyright 2025.

Licensed under the Apache L		{
			name:               "Pod with sharedvolume.io/sv/ annotation set to true",
			annotations:        map[string]string{testVolume1: "true"},
			expectSharedVolume: true,
			sharedVolumeName:   "volume1",
		},
		{
			name:               "Pod with sharedvolume.io/sv/ annotation set to false",
			annotations:        map[string]string{testVolume1: "false"},
			expectSharedVolume: false,
		},sion 2.0 (the "License");
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
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

const (
	testNamespace = "test-namespace"
)

func TestPodAnnotatorHandle(t *testing.T) {
	testCases := []struct {
		name               string
		annotations        map[string]string
		expectSharedVolume bool
		sharedVolumeName   string
	}{
		{
			name:               "Pod with sharedvolume.sv annotation",
			annotations:        map[string]string{SharedVolumeAnnotationKey: "test-volume"},
			expectSharedVolume: true,
			sharedVolumeName:   "test-volume",
		},
		{
			name:               "Pod with empty sharedvolume.sv annotation",
			annotations:        map[string]string{SharedVolumeAnnotationKey: ""},
			expectSharedVolume: false,
		},
		{
			name:               "Pod without sharedvolume annotation",
			annotations:        map[string]string{"other-annotation": "value"},
			expectSharedVolume: false,
		},
		{
			name:               "Pod with no annotations",
			annotations:        nil,
			expectSharedVolume: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create test SharedVolume if needed
			var sharedVolume *svv1alpha1.SharedVolume
			if tc.expectSharedVolume {
				sharedVolume = createTestSharedVolume(tc.sharedVolumeName)
			}

			pod := createTestPod(tc.annotations)
			result := runWebhookTest(t, pod, sharedVolume)

			if tc.expectSharedVolume {
				assert.True(t, result, "Expected shared volume to be processed")
			} else {
				assert.False(t, result, "Expected no shared volume to be processed")
			}
		})
	}
}

func createTestPod(annotations map[string]string) *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:        "test-pod",
			Namespace:   "default",
			Annotations: annotations,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "test-container",
					Image: "sharedvolume/volume-syncer:0.0.2",
				},
			},
		},
	}
}

func createTestSharedVolume(name string) *svv1alpha1.SharedVolume {
	return &svv1alpha1.SharedVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
		Spec: svv1alpha1.SharedVolumeSpec{
			VolumeSpecBase: svv1alpha1.VolumeSpecBase{
				MountPath:      "/mnt/shared",
				ReferenceValue: "test-ref",
				Storage: &svv1alpha1.StorageSpec{
					Capacity:   "10Gi",
					AccessMode: "ReadWrite",
				},
				NfsServer: &svv1alpha1.NfsServerSpec{
					Path: "/exports",
				},
			},
		},
		Status: svv1alpha1.SharedVolumeStatus{
			NfsServerAddress: "nfs-server.default.svc.cluster.local",
		},
	}
}

func runWebhookTest(t *testing.T, pod *corev1.Pod, sharedVolume *svv1alpha1.SharedVolume) bool {
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	svv1alpha1.AddToScheme(scheme)

	// Create fake client with test objects
	var objs []runtime.Object
	if sharedVolume != nil {
		objs = append(objs, sharedVolume)
	}
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

	// Marshal pod to JSON
	podBytes, err := json.Marshal(pod)
	assert.NoError(t, err)

	req := admission.Request{
		AdmissionRequest: admissionv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: podBytes},
		},
	}
	handler := &PodAnnotator{Client: fakeClient}
	resp := handler.Handle(context.Background(), req)

	// If we expect shared volume processing but no SharedVolume exists, it should fail
	expectedSharedVolumes := extractSharedVolumeNames(pod)
	if len(expectedSharedVolumes) > 0 && sharedVolume == nil {
		assert.False(t, resp.Allowed, "Expected admission to be denied when SharedVolume doesn't exist")
		return false
	}

	if len(expectedSharedVolumes) == 0 {
		assert.True(t, resp.Allowed, "Expected admission to be allowed for pods without shared volumes")
		return false // No shared volumes expected
	}

	// If we get here, we have a shared volume and expect it to be processed
	assert.True(t, resp.Allowed, "Expected admission to be allowed")

	// If the response is allowed and we have shared volumes, consider it successfully processed
	return resp.Allowed
}

func extractSharedVolumeNames(pod *corev1.Pod) []string {
	var names []string
	if pod.Annotations == nil {
		return names
	}

	// Check SharedVolume annotation: "sharedvolume.sv": "sv1,sv2,sv3"
	if sharedVolumeList, exists := pod.Annotations[SharedVolumeAnnotationKey]; exists && sharedVolumeList != "" {
		svNames := strings.Split(sharedVolumeList, ",")
		for _, svName := range svNames {
			svName = strings.TrimSpace(svName)
			if svName != "" {
				names = append(names, svName)
			}
		}
	}

	return names
}

func TestExtractSharedVolumeAnnotations(t *testing.T) {
	annotator := &PodAnnotator{}

	testCases := []struct {
		name        string
		annotations map[string]string
		expected    []SharedVolumeRef
	}{
		{
			name:        "Single shared volume annotation",
			annotations: map[string]string{SharedVolumeAnnotationKey: "volume1"},
			expected: []SharedVolumeRef{
				{
					Namespace: testNamespace,
					Name:      "volume1",
					IsCluster: false,
				},
			},
		},
		{
			name: "Multiple shared volume annotations",
			annotations: map[string]string{
				SharedVolumeAnnotationKey: "volume1,volume2",
			},
			expected: []SharedVolumeRef{
				{
					Namespace: testNamespace,
					Name:      "volume1",
					IsCluster: false,
				},
				{
					Namespace: testNamespace,
					Name:      "volume2",
					IsCluster: false,
				},
			},
		},
		{
			name:        "Single cluster shared volume annotation",
			annotations: map[string]string{ClusterSharedVolumeAnnotationKey: "csv1"},
			expected: []SharedVolumeRef{
				{
					Namespace: "",
					Name:      "csv1",
					IsCluster: true,
				},
			},
		},
		{
			name: "Multiple cluster shared volume annotations",
			annotations: map[string]string{
				ClusterSharedVolumeAnnotationKey: "csv1,csv2",
			},
			expected: []SharedVolumeRef{
				{
					Namespace: "",
					Name:      "csv1",
					IsCluster: true,
				},
				{
					Namespace: "",
					Name:      "csv2",
					IsCluster: true,
				},
			},
		},
		{
			name: "Mixed shared volume and cluster shared volume annotations",
			annotations: map[string]string{
				SharedVolumeAnnotationKey:        "volume1",
				ClusterSharedVolumeAnnotationKey: "csv1",
			},
			expected: []SharedVolumeRef{
				{
					Namespace: testNamespace,
					Name:      "volume1",
					IsCluster: false,
				},
				{
					Namespace: "",
					Name:      "csv1",
					IsCluster: true,
				},
			},
		},
		{
			name:        "Empty annotation value",
			annotations: map[string]string{SharedVolumeAnnotationKey: ""},
			expected:    []SharedVolumeRef{},
		},
		{
			name:        "No shared volume annotations",
			annotations: map[string]string{"other": "annotation"},
			expected:    []SharedVolumeRef{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pod",
					Namespace:   testNamespace,
					Annotations: tc.annotations,
				},
			}

			result := annotator.extractSharedVolumeAnnotations(pod)
			assert.ElementsMatch(t, tc.expected, result)
		})
	}
}
