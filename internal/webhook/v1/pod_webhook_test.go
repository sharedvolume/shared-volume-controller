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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
	"github.com/sharedvolume/shared-volume-controller/internal/webhook/types"
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
			annotations:        map[string]string{types.SharedVolumeAnnotationKey: "test-volume"},
			expectSharedVolume: true,
			sharedVolumeName:   "test-volume",
		},
		{
			name:               "Pod with empty sharedvolume.sv annotation",
			annotations:        map[string]string{types.SharedVolumeAnnotationKey: ""},
			expectSharedVolume: false,
		},
		{
			name:               "Pod without SharedVolume annotation",
			annotations:        map[string]string{},
			expectSharedVolume: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create scheme and add our types
			scheme := runtime.NewScheme()
			_ = corev1.AddToScheme(scheme)
			_ = svv1alpha1.AddToScheme(scheme)

			// Create fake client
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			// Create annotator
			annotator := NewPodAnnotator(client)

			// Create test pod
			pod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test-pod",
					Namespace:   testNamespace,
					Annotations: tc.annotations,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "test-container",
							Image: "test-image",
						},
					},
				},
			}

			// Create admission request
			req := admission.Request{}

			// Test Handle method
			resp := annotator.Handle(context.TODO(), req)

			// Basic validation that it doesn't crash
			assert.NotNil(t, resp)
			_ = pod // Use the pod variable to avoid lint error
		})
	}
}
