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

package base

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/types"
	"github.com/sharedvolume/shared-volume-controller/internal/controller/utils"
)

// StatusManager manages volume status updates
type StatusManager struct {
	Client       client.Client
	RetryManager *utils.RetryManager
}

// NewStatusManager creates a new StatusManager instance
func NewStatusManager(client client.Client) *StatusManager {
	return &StatusManager{
		Client:       client,
		RetryManager: utils.NewRetryManager(client),
	}
}

// UpdateVolumeStatus updates the status fields and returns true if any field was changed
func (s *StatusManager) UpdateVolumeStatus(volumeObj types.VolumeObject, nfsReady bool, phase string, message string) bool {
	statusChanged := false

	// Update phase if changed and phase is not empty (empty phase means don't update)
	if phase != "" && volumeObj.GetPhase() != phase {
		volumeObj.SetPhase(phase)
		statusChanged = true
	}

	// Update message if changed
	if volumeObj.GetMessage() != message {
		volumeObj.SetMessage(message)
		statusChanged = true
	}

	// Set NFS server address if available and not already set
	spec := volumeObj.GetVolumeSpec()
	if nfsReady && volumeObj.GetNfsServerAddress() == "" && spec.NfsServer != nil && spec.NfsServer.URL != "" {
		volumeObj.SetNfsServerAddress(spec.NfsServer.URL)
		statusChanged = true
	}

	return statusChanged
}

// UpdateVolumeStatusSimple updates basic status fields (phase and message) and returns true if any field was changed
func (s *StatusManager) UpdateVolumeStatusSimple(volumeObj types.VolumeObject, phase string, message string) bool {
	statusChanged := false

	// Update phase if changed
	if volumeObj.GetPhase() != phase {
		volumeObj.SetPhase(phase)
		statusChanged = true
	}

	// Update message if changed
	if volumeObj.GetMessage() != message {
		volumeObj.SetMessage(message)
		statusChanged = true
	}

	return statusChanged
}

// DeterminePhase determines the correct phase based on NFS and ReplicaSet readiness
func (s *StatusManager) DeterminePhase(nfsReady, replicaSetReady bool) string {
	if nfsReady && replicaSetReady {
		return types.VolumePhaseReady
	} else if nfsReady && !replicaSetReady {
		return "Preparing"
	} else {
		return "Pending"
	}
}

// UpdateReadinessStatus updates the volume status based on NFS and ReplicaSet readiness
func (s *StatusManager) UpdateReadinessStatus(ctx context.Context, volumeObj types.VolumeObject, nfsReady, replicaSetReady bool,
	onReady func(ctx context.Context, volumeObj types.VolumeObject) error,
	onNotReady func(ctx context.Context, volumeObj types.VolumeObject) error) error {
	log := logf.FromContext(ctx)

	phase := s.DeterminePhase(nfsReady, replicaSetReady)
	overallReady := phase == types.VolumePhaseReady
	spec := volumeObj.GetVolumeSpec()

	// Determine the appropriate message
	var message string
	switch phase {
	case types.VolumePhaseReady:
		message = "Volume is ready for use"
	case "Preparing":
		message = "NFS server is ready, waiting for ReplicaSet to be ready"
	case "Pending":
		if !nfsReady {
			message = "Waiting for NFS server to be ready"
		} else {
			message = "Waiting for resources to be ready"
		}
	}

	// Check if we need to update NFS server address
	needsNfsAddressUpdate := nfsReady && spec.NfsServer != nil && spec.NfsServer.URL != "" && volumeObj.GetNfsServerAddress() != spec.NfsServer.URL

	// Only update status if something actually changed
	if volumeObj.GetPhase() == phase && volumeObj.GetMessage() == message && !needsNfsAddressUpdate {
		// Status is already up-to-date, just execute callbacks
		if overallReady && onReady != nil {
			if err := onReady(ctx, volumeObj); err != nil {
				log.Error(err, "Error executing ready callback")
				return err
			}
		} else if !overallReady && onNotReady != nil {
			if err := onNotReady(ctx, volumeObj); err != nil {
				log.Error(err, "Error executing not ready callback")
				return err
			}
		}
		return nil
	}

	err := s.RetryManager.UpdateStatusWithRetry(ctx, volumeObj, func(obj client.Object) {
		if volumeObj, ok := obj.(types.VolumeObject); ok {
			volumeObj.SetPhase(phase)
			volumeObj.SetMessage(message)

			// Set NFS server address if available and NFS is ready
			if nfsReady && spec.NfsServer != nil && spec.NfsServer.URL != "" {
				volumeObj.SetNfsServerAddress(spec.NfsServer.URL)
			}
		}
	})

	if err != nil {
		log.Error(err, "Failed to update volume status")
		return err
	}

	// Get the latest version to update our local reference
	latestObj := volumeObj.DeepCopyObject().(types.VolumeObject)
	if err := s.Client.Get(ctx, client.ObjectKeyFromObject(volumeObj), latestObj); err != nil {
		return err
	}

	// Copy the latest version back to the original object
	volumeObj.SetResourceVersion(latestObj.GetResourceVersion())
	volumeObj.SetPhase(latestObj.GetPhase())
	volumeObj.SetMessage(latestObj.GetMessage())

	// Execute callbacks based on readiness state
	if overallReady && onReady != nil {
		if err := onReady(ctx, volumeObj); err != nil {
			log.Error(err, "Error executing ready callback")
			return err
		}
	} else if !overallReady && onNotReady != nil {
		if err := onNotReady(ctx, volumeObj); err != nil {
			log.Error(err, "Error executing not ready callback")
			return err
		}
	}

	return nil
}

// UpdateStatus updates the volume status with retry logic
func (s *StatusManager) UpdateStatus(ctx context.Context, volumeObj types.VolumeObject, nfsReady bool, phase string, message string) error {
	return s.RetryManager.UpdateStatusWithRetry(ctx, volumeObj, func(obj client.Object) {
		if volObj, ok := obj.(types.VolumeObject); ok {
			s.UpdateVolumeStatus(volObj, nfsReady, phase, message)
		}
	})
}

// UpdateSimpleStatus updates the volume status with just phase and message
func (s *StatusManager) UpdateSimpleStatus(ctx context.Context, volumeObj types.VolumeObject, phase string, message string) error {
	// Only update if the status actually changed
	if volumeObj.GetPhase() == phase && volumeObj.GetMessage() == message {
		return nil // No change needed
	}

	return s.RetryManager.UpdateStatusWithRetry(ctx, volumeObj, func(obj client.Object) {
		if volObj, ok := obj.(types.VolumeObject); ok {
			s.UpdateVolumeStatusSimple(volObj, phase, message)
		}
	})
}

// SetInitializingStatus sets the volume to initializing state
func (s *StatusManager) SetInitializingStatus(ctx context.Context, volumeObj types.VolumeObject) error {
	return s.UpdateSimpleStatus(ctx, volumeObj, types.VolumePhaseInitializing, "Initializing volume resources")
}

// SetCreatingNfsServerStatus sets the volume to creating NFS server state
func (s *StatusManager) SetCreatingNfsServerStatus(ctx context.Context, volumeObj types.VolumeObject) error {
	return s.UpdateSimpleStatus(ctx, volumeObj, types.VolumePhaseCreatingNfsServer, "Creating NFS server")
}

// SetWaitingForNfsServerStatus sets the volume to waiting for NFS server state
func (s *StatusManager) SetWaitingForNfsServerStatus(ctx context.Context, volumeObj types.VolumeObject) error {
	return s.UpdateSimpleStatus(ctx, volumeObj, types.VolumePhaseWaitingForNfsServer, "Waiting for NFS server to be ready")
}

// SetCreatingResourcesStatus sets the volume to creating resources state
func (s *StatusManager) SetCreatingResourcesStatus(ctx context.Context, volumeObj types.VolumeObject) error {
	return s.UpdateSimpleStatus(ctx, volumeObj, types.VolumePhaseCreatingResources, "Creating volume resources")
}

// SetErrorStatus sets the volume to error state with a specific message
func (s *StatusManager) SetErrorStatus(ctx context.Context, volumeObj types.VolumeObject, errorMsg string) error {
	return s.UpdateSimpleStatus(ctx, volumeObj, types.VolumePhaseError, errorMsg)
}
