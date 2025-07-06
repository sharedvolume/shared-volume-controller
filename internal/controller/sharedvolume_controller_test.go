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

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

var _ = Describe("SharedVolume Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		sharedvolume := &svv1alpha1.SharedVolume{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind SharedVolume")
			err := k8sClient.Get(ctx, typeNamespacedName, sharedvolume)
			if err != nil && errors.IsNotFound(err) {
				resource := &svv1alpha1.SharedVolume{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: svv1alpha1.SharedVolumeSpec{
						VolumeSpecBase: svv1alpha1.VolumeSpecBase{
							MountPath: "/mnt/shared",
							Storage: &svv1alpha1.StorageSpec{
								Capacity:   "1Gi",
								AccessMode: "ReadWrite",
							},
							NfsServer: &svv1alpha1.NfsServerSpec{
								Image: "nfs-server:latest",
								Path:  "/exports",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &svv1alpha1.SharedVolume{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance SharedVolume")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &SharedVolumeReconciler{
				VolumeControllerBase: NewVolumeControllerBase(k8sClient, k8sClient.Scheme(), "shared-volume-controller", nil),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Context("When handling NfsServer Path field", func() {
		It("should default NfsServer Path to '/' when not provided", func() {
			// Create a SharedVolume with NfsServer that doesn't have Path field
			sharedVolume := &svv1alpha1.SharedVolume{
				Spec: svv1alpha1.SharedVolumeSpec{
					VolumeSpecBase: svv1alpha1.VolumeSpecBase{
						MountPath: "/data",
						Storage: &svv1alpha1.StorageSpec{
							Capacity:   "1Gi",
							AccessMode: "ReadWrite",
						},
						NfsServer: &svv1alpha1.NfsServerSpec{
							Name:      "test-nfs",
							Namespace: "default",
							URL:       "test-nfs.default.svc.cluster.local",
							// Path is intentionally not set
						},
					},
				},
			}

			// Call FillAndValidateSpec and validate results
			generateNfsServer := false // We're already providing NfsServer
			base := NewVolumeControllerBase(k8sClient, k8sClient.Scheme(), "shared-volume-controller", nil)
			err := base.FillAndValidateSpec(sharedVolume, generateNfsServer)
			Expect(err).NotTo(HaveOccurred())

			// Verify Path is defaulted to "/"
			Expect(sharedVolume.Spec.NfsServer.Path).To(Equal("/"))
		})

		It("should preserve existing NfsServer Path when provided", func() {
			// Create a SharedVolume with NfsServer that has a custom Path
			customPath := "/exports"
			sharedVolume := &svv1alpha1.SharedVolume{
				Spec: svv1alpha1.SharedVolumeSpec{
					VolumeSpecBase: svv1alpha1.VolumeSpecBase{
						MountPath: "/data",
						Storage: &svv1alpha1.StorageSpec{
							Capacity:   "1Gi",
							AccessMode: "ReadWrite",
						},
						NfsServer: &svv1alpha1.NfsServerSpec{
							Name:      "test-nfs",
							Namespace: "default",
							URL:       "test-nfs.default.svc.cluster.local",
							Path:      customPath,
						},
					},
				},
			}

			// Call FillAndValidateSpec and validate results
			generateNfsServer := false // We're already providing NfsServer
			base := NewVolumeControllerBase(k8sClient, k8sClient.Scheme(), "shared-volume-controller", nil)
			err := base.FillAndValidateSpec(sharedVolume, generateNfsServer)
			Expect(err).NotTo(HaveOccurred())

			// Verify Path is preserved
			Expect(sharedVolume.Spec.NfsServer.Path).To(Equal(customPath))
		})
	})
})
