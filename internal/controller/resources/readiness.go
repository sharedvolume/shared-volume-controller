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

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/types"
)

// ReadinessManager manages readiness checks for all volume resources
type ReadinessManager struct {
	Client            client.Client
	NfsManager        *NfsServerManager
	PVManager         *PersistentVolumeManager
	PVCManager        *PersistentVolumeClaimManager
	ReplicaSetManager *ReplicaSetManager
	ServiceManager    *ServiceManager
}

// NewReadinessManager creates a new ReadinessManager instance
func NewReadinessManager(client client.Client) *ReadinessManager {
	return &ReadinessManager{
		Client:            client,
		NfsManager:        NewNfsServerManager(client),
		PVManager:         NewPersistentVolumeManager(client),
		PVCManager:        NewPersistentVolumeClaimManager(client),
		ReplicaSetManager: NewReplicaSetManager(client),
		ServiceManager:    NewServiceManager(client),
	}
}

// CheckAllResources checks the readiness of all resources
func (r *ReadinessManager) CheckAllResources(ctx context.Context, volumeObj types.VolumeObject, namespace string) (*ReadinessResult, error) {
	log := logf.FromContext(ctx)

	result := &ReadinessResult{}
	var err error

	// Check NFS server
	result.NfsReady, err = r.NfsManager.IsReady(ctx, volumeObj, namespace)
	if err != nil {
		log.Error(err, "Failed to check NFS server readiness")
		return result, err
	}

	// Check PV
	result.PVReady, err = r.PVManager.IsReady(ctx, volumeObj, namespace)
	if err != nil {
		log.Error(err, "Failed to check PV readiness")
		return result, err
	}

	// Check PVC
	result.PVCReady, err = r.PVCManager.IsReady(ctx, volumeObj, namespace)
	if err != nil {
		log.Error(err, "Failed to check PVC readiness")
		return result, err
	}

	// Check ReplicaSet
	result.ReplicaSetReady, err = r.ReplicaSetManager.IsReady(ctx, volumeObj, namespace)
	if err != nil {
		log.Error(err, "Failed to check ReplicaSet readiness")
		return result, err
	}

	// Check Service
	result.ServiceReady, err = r.ServiceManager.IsReady(ctx, volumeObj, namespace)
	if err != nil {
		log.Error(err, "Failed to check Service readiness")
		return result, err
	}

	// Determine overall readiness
	result.OverallReady = result.NfsReady && result.PVReady && result.PVCReady && result.ReplicaSetReady && result.ServiceReady

	log.Info("Resource readiness check complete",
		"nfs", result.NfsReady,
		"pv", result.PVReady,
		"pvc", result.PVCReady,
		"replicaSet", result.ReplicaSetReady,
		"service", result.ServiceReady,
		"overall", result.OverallReady)

	return result, nil
}

// CheckCoreResources checks only the core resources (NFS and ReplicaSet)
func (r *ReadinessManager) CheckCoreResources(ctx context.Context, volumeObj types.VolumeObject, namespace string) (bool, bool, error) {
	nfsReady, err := r.NfsManager.IsReady(ctx, volumeObj, namespace)
	if err != nil {
		return false, false, err
	}

	replicaSetReady, err := r.ReplicaSetManager.IsReady(ctx, volumeObj, namespace)
	if err != nil {
		return false, false, err
	}

	return nfsReady, replicaSetReady, nil
}

// IsNfsServerReady checks if the NFS server is ready
func (r *ReadinessManager) IsNfsServerReady(ctx context.Context, volumeObj types.VolumeObject, namespace string) (bool, error) {
	return r.NfsManager.IsReady(ctx, volumeObj, namespace)
}

// IsReplicaSetReady checks if the ReplicaSet is ready
func (r *ReadinessManager) IsReplicaSetReady(ctx context.Context, volumeObj types.VolumeObject, namespace string) (bool, error) {
	return r.ReplicaSetManager.IsReady(ctx, volumeObj, namespace)
}

// IsPVCReady checks if the PVC is ready (bound)
func (r *ReadinessManager) IsPVCReady(ctx context.Context, volumeObj types.VolumeObject, namespace string) (bool, error) {
	return r.PVCManager.IsReady(ctx, volumeObj, namespace)
}

// WaitForNfsServer waits for the NFS server to become ready
func (r *ReadinessManager) WaitForNfsServer(ctx context.Context, volumeObj types.VolumeObject, namespace string) error {
	ready, err := r.IsNfsServerReady(ctx, volumeObj, namespace)
	if err != nil {
		return err
	}

	if !ready {
		return &NotReadyError{Resource: "NFS Server", Message: "NFS server is not ready yet"}
	}

	return nil
}

// WaitForPVC waits for the PVC to become bound
func (r *ReadinessManager) WaitForPVC(ctx context.Context, volumeObj types.VolumeObject, namespace string) error {
	ready, err := r.IsPVCReady(ctx, volumeObj, namespace)
	if err != nil {
		return err
	}

	if !ready {
		return &NotReadyError{Resource: "PVC", Message: "PVC is not bound yet"}
	}

	return nil
}

// WaitForReplicaSet waits for the ReplicaSet to become ready
func (r *ReadinessManager) WaitForReplicaSet(ctx context.Context, volumeObj types.VolumeObject, namespace string) error {
	ready, err := r.IsReplicaSetReady(ctx, volumeObj, namespace)
	if err != nil {
		return err
	}

	if !ready {
		return &NotReadyError{Resource: "ReplicaSet", Message: "ReplicaSet is not ready yet"}
	}

	return nil
}

// ReadinessResult holds the readiness status of all resources
type ReadinessResult struct {
	NfsReady        bool
	PVReady         bool
	PVCReady        bool
	ReplicaSetReady bool
	ServiceReady    bool
	OverallReady    bool
}

// NotReadyError represents an error when a resource is not ready
type NotReadyError struct {
	Resource string
	Message  string
}

func (e *NotReadyError) Error() string {
	return e.Message
}

// IsNotReady checks if an error is a NotReadyError
func IsNotReady(err error) bool {
	_, ok := err.(*NotReadyError)
	return ok
}
