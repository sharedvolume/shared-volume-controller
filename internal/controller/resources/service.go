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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/types"
	"github.com/sharedvolume/shared-volume-controller/internal/controller/utils"
)

// ServiceManager manages Service resources
type ServiceManager struct {
	Client  client.Client
	NameGen *utils.NameGenerator
}

// NewServiceManager creates a new ServiceManager instance
func NewServiceManager(client client.Client) *ServiceManager {
	return &ServiceManager{
		Client:  client,
		NameGen: utils.NewNameGenerator(),
	}
}

// Reconcile ensures the Service exists and is in the desired state
func (s *ServiceManager) Reconcile(ctx context.Context, volumeObj types.VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Generate Service name
	serviceName := s.GenerateServiceName(spec.ReferenceValue)
	replicaSetName := spec.ReferenceValue

	// Check if Service already exists
	existingSvc := &corev1.Service{}
	err := s.Client.Get(ctx, client.ObjectKey{
		Name:      serviceName,
		Namespace: namespace,
	}, existingSvc)

	if err == nil {
		// Service already exists, check if it has the correct configuration
		if s.needsUpdate(existingSvc) {
			// Delete the existing Service so it can be recreated with correct config
			log.Info("Deleting Service with incorrect configuration", "name", serviceName)
			if err := s.Client.Delete(ctx, existingSvc); err != nil {
				log.Error(err, "Failed to delete Service for update", "name", serviceName)
				return err
			}
			// Continue to create a new one below
		} else {
			// Service exists and has correct configuration
			log.Info("Service already exists with correct configuration", "name", serviceName)
			return nil
		}
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to check if Service exists")
		return err
	}

	// Create the Service
	return s.CreateService(ctx, volumeObj, namespace, serviceName, replicaSetName)
}

// IsReady checks if the Service is ready
func (s *ServiceManager) IsReady(ctx context.Context, volumeObj types.VolumeObject, namespace string) (bool, error) {
	spec := volumeObj.GetVolumeSpec()
	serviceName := s.GenerateServiceName(spec.ReferenceValue)

	svc := &corev1.Service{}
	err := s.Client.Get(ctx, client.ObjectKey{
		Name:      serviceName,
		Namespace: namespace,
	}, svc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Service is ready if it exists and has correct configuration
	return !s.needsUpdate(svc), nil
}

// CreateService creates a new Service
func (s *ServiceManager) CreateService(ctx context.Context, volumeObj types.VolumeObject, namespace, serviceName, replicaSetName string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
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
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": replicaSetName,
			},
			Ports: []corev1.ServicePort{
				{
					Port:       8080,
					TargetPort: intstr.FromInt(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	// Create the Service
	if err := s.Client.Create(ctx, svc); err != nil {
		log.Error(err, "Failed to create Service", "name", serviceName)
		return err
	}

	log.Info("Created Service", "name", serviceName)
	return nil
}

// needsUpdate checks if the Service needs to be updated
func (s *ServiceManager) needsUpdate(svc *corev1.Service) bool {
	// Check if service port is correct (should be 8080)
	if len(svc.Spec.Ports) > 0 {
		currentPort := svc.Spec.Ports[0].Port
		if currentPort != 8080 {
			return true
		}
	}
	return false
}

// GenerateServiceName generates a Service name based on reference value
func (s *ServiceManager) GenerateServiceName(referenceValue string) string {
	return referenceValue
}

// DeleteService deletes a Service
func (s *ServiceManager) DeleteService(ctx context.Context, serviceName, namespace string) error {
	log := logf.FromContext(ctx)

	svc := &corev1.Service{}
	err := s.Client.Get(ctx, client.ObjectKey{
		Name:      serviceName,
		Namespace: namespace,
	}, svc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Service already deleted", "name", serviceName, "namespace", namespace)
			return nil
		}
		return err
	}

	if err := s.Client.Delete(ctx, svc); err != nil {
		log.Error(err, "Failed to delete Service", "name", serviceName)
		return err
	}

	log.Info("Deleted Service", "name", serviceName, "namespace", namespace)
	return nil
}

// GetService retrieves a Service by name and namespace
func (s *ServiceManager) GetService(ctx context.Context, serviceName, namespace string) (*corev1.Service, error) {
	svc := &corev1.Service{}
	err := s.Client.Get(ctx, client.ObjectKey{
		Name:      serviceName,
		Namespace: namespace,
	}, svc)
	return svc, err
}

// UpdateServiceStatus updates the service status in the volume object
func (s *ServiceManager) UpdateServiceStatus(volumeObj types.VolumeObject, serviceName string) {
	if volumeObj.GetServiceName() != serviceName {
		volumeObj.SetServiceName(serviceName)
	}
}
