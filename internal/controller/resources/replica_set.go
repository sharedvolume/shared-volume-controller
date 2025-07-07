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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/types"
	"github.com/sharedvolume/shared-volume-controller/internal/controller/utils"
)

// ReplicaSetManager manages ReplicaSet resources
type ReplicaSetManager struct {
	Client  client.Client
	NameGen *utils.NameGenerator
}

// NewReplicaSetManager creates a new ReplicaSetManager instance
func NewReplicaSetManager(client client.Client) *ReplicaSetManager {
	return &ReplicaSetManager{
		Client:  client,
		NameGen: utils.NewNameGenerator(),
	}
}

// Reconcile ensures the ReplicaSet exists and is in the desired state
func (r *ReplicaSetManager) Reconcile(ctx context.Context, volumeObj types.VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Generate ReplicaSet name
	replicaSetName := r.GenerateReplicaSetName(spec.ReferenceValue)
	pvcName := spec.ReferenceValue

	// Check if ReplicaSet already exists
	existingRS := &appsv1.ReplicaSet{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Name:      replicaSetName,
		Namespace: namespace,
	}, existingRS)

	if err == nil {
		// ReplicaSet already exists, check if it has the correct configuration
		if r.needsUpdate(existingRS) {
			// Delete the existing ReplicaSet so it can be recreated with correct config
			log.Info("Deleting ReplicaSet with incorrect configuration", "name", replicaSetName)
			if err := r.Client.Delete(ctx, existingRS); err != nil {
				log.Error(err, "Failed to delete ReplicaSet for update", "name", replicaSetName)
				return err
			}
			// Continue to create a new one below
		} else {
			// ReplicaSet exists and has correct configuration
			log.Info("ReplicaSet already exists with correct configuration", "name", replicaSetName)
			return nil
		}
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to check if ReplicaSet exists")
		return err
	}

	// Create the ReplicaSet
	return r.CreateReplicaSet(ctx, volumeObj, namespace, replicaSetName, pvcName)
}

// IsReady checks if the ReplicaSet is ready
func (r *ReplicaSetManager) IsReady(ctx context.Context, volumeObj types.VolumeObject, namespace string) (bool, error) {
	spec := volumeObj.GetVolumeSpec()
	replicaSetName := r.GenerateReplicaSetName(spec.ReferenceValue)

	rs := &appsv1.ReplicaSet{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Name:      replicaSetName,
		Namespace: namespace,
	}, rs)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Check if ReplicaSet has desired number of ready replicas
	return rs.Status.ReadyReplicas == *rs.Spec.Replicas && rs.Status.ReadyReplicas > 0, nil
}

// CreateReplicaSet creates a new ReplicaSet
func (r *ReplicaSetManager) CreateReplicaSet(ctx context.Context, volumeObj types.VolumeObject, namespace, replicaSetName, pvcName string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	var replicas int32 = 1
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      replicaSetName,
			Namespace: namespace,
			Labels: map[string]string{
				"app":                          replicaSetName,
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
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": replicaSetName,
				},
			},
			Template: r.createPodTemplate(volumeObj, namespace, replicaSetName, pvcName),
		},
	}

	// Create the ReplicaSet
	if err := r.Client.Create(ctx, rs); err != nil {
		log.Error(err, "Failed to create ReplicaSet", "name", replicaSetName)
		return err
	}

	log.Info("Created ReplicaSet", "name", replicaSetName, "referenceID", spec.ReferenceValue)
	return nil
}

// createPodTemplate creates the pod template for the ReplicaSet
func (r *ReplicaSetManager) createPodTemplate(volumeObj types.VolumeObject, namespace, replicaSetName, pvcName string) corev1.PodTemplateSpec {
	spec := volumeObj.GetVolumeSpec()

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				"app": replicaSetName,
			},
		},
		Spec: corev1.PodSpec{
			InitContainers: []corev1.Container{
				{
					Name:  "setup-folders",
					Image: "sharedvolume/volume-syncer:0.0.2",
					Command: []string{
						"sh",
						"-c", fmt.Sprintf("mkdir -p /nfs/%s-%s && echo 'sv-sample-file' > /nfs/%s-%s/.sv && echo 'Created folder /nfs/%s-%s with .sv file'",
							volumeObj.GetName(), namespace,
							volumeObj.GetName(), namespace,
							volumeObj.GetName(), namespace),
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "shared-volume",
							MountPath: "/nfs",
						},
					},
				},
			},
			Containers: []corev1.Container{
				{
					Name:  spec.ReferenceValue + "-syncer",
					Image: "sharedvolume/volume-syncer:0.0.2",
					Ports: []corev1.ContainerPort{
						{
							ContainerPort: 8080,
							Protocol:      corev1.ProtocolTCP,
						},
					},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "shared-volume",
							MountPath: "/nfs",
						},
					},
				},
			},
			Volumes: []corev1.Volume{
				{
					Name: "shared-volume",
					VolumeSource: corev1.VolumeSource{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
							ClaimName: pvcName,
						},
					},
				},
			},
		},
	}
}

// needsUpdate checks if the ReplicaSet needs to be updated
func (r *ReplicaSetManager) needsUpdate(rs *appsv1.ReplicaSet) bool {
	// Check if container port is correct (should be 8080)
	if len(rs.Spec.Template.Spec.Containers) > 0 &&
		len(rs.Spec.Template.Spec.Containers[0].Ports) > 0 {
		currentPort := rs.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort
		if currentPort != 8080 {
			return true
		}
	}
	return false
}

// GenerateReplicaSetName generates a ReplicaSet name based on reference value
func (r *ReplicaSetManager) GenerateReplicaSetName(referenceValue string) string {
	return referenceValue
}

// DeleteReplicaSet deletes a ReplicaSet
func (r *ReplicaSetManager) DeleteReplicaSet(ctx context.Context, replicaSetName, namespace string) error {
	log := logf.FromContext(ctx)

	rs := &appsv1.ReplicaSet{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Name:      replicaSetName,
		Namespace: namespace,
	}, rs)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("ReplicaSet already deleted", "name", replicaSetName, "namespace", namespace)
			return nil
		}
		return err
	}

	if err := r.Client.Delete(ctx, rs); err != nil {
		log.Error(err, "Failed to delete ReplicaSet", "name", replicaSetName)
		return err
	}

	log.Info("Deleted ReplicaSet", "name", replicaSetName, "namespace", namespace)
	return nil
}

// GetReplicaSet retrieves a ReplicaSet by name and namespace
func (r *ReplicaSetManager) GetReplicaSet(ctx context.Context, replicaSetName, namespace string) (*appsv1.ReplicaSet, error) {
	rs := &appsv1.ReplicaSet{}
	err := r.Client.Get(ctx, client.ObjectKey{
		Name:      replicaSetName,
		Namespace: namespace,
	}, rs)
	return rs, err
}

// ScaleReplicaSet scales a ReplicaSet to the specified number of replicas
func (r *ReplicaSetManager) ScaleReplicaSet(ctx context.Context, replicaSetName, namespace string, replicas int32) error {
	log := logf.FromContext(ctx)

	rs, err := r.GetReplicaSet(ctx, replicaSetName, namespace)
	if err != nil {
		return err
	}

	if *rs.Spec.Replicas == replicas {
		log.V(1).Info("ReplicaSet already has desired replica count", "name", replicaSetName, "replicas", replicas)
		return nil
	}

	rs.Spec.Replicas = &replicas
	if err := r.Client.Update(ctx, rs); err != nil {
		log.Error(err, "Failed to scale ReplicaSet", "name", replicaSetName, "replicas", replicas)
		return err
	}

	log.Info("Scaled ReplicaSet", "name", replicaSetName, "replicas", replicas)
	return nil
}
