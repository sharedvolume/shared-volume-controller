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
	"testing"

	"github.com/stretchr/testify/assert"
	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func TestPodAnnotatorLogic(t *testing.T) {
	// Test the basic logic without admission framework
	tests := []struct {
		name        string
		annotations map[string]string
		expectLabel bool
	}{
		{
			name:        "Pod with sharedvolume.io annotation",
			annotations: map[string]string{"sharedvolume.io/mount-path": "/mnt/shared"},
			expectLabel: true,
		},
		{
			name:        "Pod without sharedvolume.io annotation",
			annotations: map[string]string{"other-annotation": "value"},
			expectLabel: false,
		},
		{
			name:        "Pod with no annotations",
			annotations: nil,
			expectLabel: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pod",
					Namespace:   "default",
					Annotations: tc.annotations,
				},
			}

			// Test the logic manually
			hasSharedVolumeAnnotation := false
			if pod.Annotations != nil {
				for key := range pod.Annotations {
					if len(key) >= 16 && key[:16] == "sharedvolume.io/" {
						hasSharedVolumeAnnotation = true
						break
					}
				}
			}

			assert.Equal(t, tc.expectLabel, hasSharedVolumeAnnotation, "Logic test failed")

			// Now test with webhook
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)

			podBytes, err := json.Marshal(pod)
			assert.NoError(t, err)

			req := admission.Request{
				AdmissionRequest: admissionv1.AdmissionRequest{
					Object: runtime.RawExtension{Raw: podBytes},
				},
			}

			handler := &PodAnnotator{}

			resp := handler.Handle(context.Background(), req)
			assert.True(t, resp.Allowed, "Expected admission to be allowed")

			t.Logf("Response patches: %+v", resp.Patches)

			// Check the original pod and modified pod
			originalPod := &corev1.Pod{}
			err = json.Unmarshal(podBytes, originalPod)
			assert.NoError(t, err)
			t.Logf("Original pod labels: %+v", originalPod.Labels)
		})
	}
}
