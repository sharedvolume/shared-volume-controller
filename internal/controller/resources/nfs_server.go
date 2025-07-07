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
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nfsv1alpha1 "github.com/sharedvolume/nfs-server-controller/api/v1alpha1"
	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
	"github.com/sharedvolume/shared-volume-controller/internal/controller/types"
	"github.com/sharedvolume/shared-volume-controller/internal/controller/utils"
)

// NfsServerManager manages NFS server resources
type NfsServerManager struct {
	Client    client.Client
	RandomGen *utils.RandomGenerator
}

// NewNfsServerManager creates a new NfsServerManager instance
func NewNfsServerManager(client client.Client) *NfsServerManager {
	return &NfsServerManager{
		Client:    client,
		RandomGen: utils.NewRandomGenerator(),
	}
}

// Reconcile ensures the NFS server exists and is in the desired state
func (n *NfsServerManager) Reconcile(ctx context.Context, volumeObj types.VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// If no NFS server configured, skip
	if spec.NfsServer == nil || spec.NfsServer.Name == "" {
		log.V(1).Info("No NFS server configured, skipping reconciliation")
		return nil
	}

	return n.CreateAndOwnNfsServer(ctx, volumeObj, namespace)
}

// IsReady checks if the NFS server is ready
func (n *NfsServerManager) IsReady(ctx context.Context, volumeObj types.VolumeObject, namespace string) (bool, error) {
	ready, _, _, err := n.CheckNfsServerStatus(ctx, volumeObj, namespace)
	return ready, err
}

// CreateAndOwnNfsServer creates an NfsServer resource and sets ownership
func (n *NfsServerManager) CreateAndOwnNfsServer(ctx context.Context, volumeObj types.VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Check if NfsServer already exists to prevent duplicates
	existingNfsServer := &nfsv1alpha1.NfsServer{}
	err := n.Client.Get(ctx, client.ObjectKey{
		Name:      spec.NfsServer.Name,
		Namespace: namespace,
	}, existingNfsServer)

	if err == nil {
		log.Info("NfsServer already exists, skipping creation", "name", spec.NfsServer.Name)
		return nil
	}

	if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to check if NfsServer exists")
		return err
	}

	nfsServer := &nfsv1alpha1.NfsServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.NfsServer.Name,
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
		Spec: nfsv1alpha1.NfsServerSpec{
			Storage: nfsv1alpha1.StorageSpec{
				Capacity:         spec.Storage.Capacity,
				StorageClassName: spec.StorageClassName,
			},
			Image: spec.NfsServer.Image,
		},
	}

	log.Info("Creating NfsServer", "name", nfsServer.Name, "namespace", nfsServer.Namespace)
	return n.Client.Create(ctx, nfsServer)
}

// GenerateAndCreateNfsServer creates an NFS server with default configuration
func (n *NfsServerManager) GenerateAndCreateNfsServer(ctx context.Context, volumeObj types.VolumeObject, resourcePrefix string, namespace string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Generate reference value if not set
	if spec.ReferenceValue == "" {
		spec.ReferenceValue = resourcePrefix + "-" + n.RandomGen.RandString(12)
	}

	// Generate NFS server configuration
	nfsServerName := "nfs-" + spec.ReferenceValue
	spec.NfsServer = &svv1alpha1.NfsServerSpec{
		Name:      nfsServerName,
		Namespace: namespace,
		URL:       nfsServerName + "." + namespace + ".svc.cluster.local",
		Path:      "/", // Default path, will be used as the actual share path
	}

	log.Info("Generating NfsServer", "name", nfsServerName, "referenceValue", spec.ReferenceValue)

	// Update the VolumeObject with the generated NFS server spec and reference value
	if err := n.Client.Update(ctx, volumeObj); err != nil {
		log.Error(err, "Failed to update VolumeObject with generated NFS server spec")
		return ctrl.Result{}, err
	}

	// Create the NFS server
	if err := n.CreateAndOwnNfsServer(ctx, volumeObj, namespace); err != nil {
		log.Error(err, "Failed to create NFS server")
		return ctrl.Result{}, err
	}

	// Requeue to allow the NFS server to be created and become ready
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

// CheckNfsServerStatus checks if the NfsServer exists and is ready
func (n *NfsServerManager) CheckNfsServerStatus(ctx context.Context, volumeObj types.VolumeObject, namespace string) (bool, string, string, error) {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	nfsServer := &nfsv1alpha1.NfsServer{}
	err := n.Client.Get(ctx, client.ObjectKey{
		Name:      spec.NfsServer.Name,
		Namespace: namespace,
	}, nfsServer)

	if client.IgnoreNotFound(err) != nil {
		log.Error(err, "Error getting NfsServer")
		return false, types.VolumePhaseError, "Failed to get NfsServer: " + err.Error(), err
	}

	if err != nil { // Not found
		message := "NfsServer not found, waiting for creation"
		log.Info(message,
			"name", spec.NfsServer.Name,
			"namespace", namespace)
		return false, "Pending", message, nil
	}

	if !nfsServer.Status.Ready {
		message := "NfsServer exists but is not ready yet"
		log.Info(message,
			"name", spec.NfsServer.Name,
			"status", nfsServer.Status.Phase)
		return false, "Pending", message, nil
	}

	// NFS server is ready
	return true, "", "NfsServer is ready", nil
}

// ReconcileComplete handles the complete NFS server lifecycle
func (n *NfsServerManager) ReconcileComplete(ctx context.Context, volumeObj types.VolumeObject, namespace string, generateNfsServer bool, resourcePrefix string) (ctrl.Result, error) {
	// If we need to generate an NfsServer, handle it here
	if generateNfsServer {
		result, err := n.GenerateAndCreateNfsServer(ctx, volumeObj, resourcePrefix, namespace)
		if err != nil {
			return result, err
		}
		// If still in progress, return the result
		if result.RequeueAfter > 0 {
			return result, nil
		}
	}

	// Check if NFS server exists and is ready
	ready, _, message, err := n.CheckNfsServerStatus(ctx, volumeObj, namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// If NFS server not ready yet, requeue
	if !ready {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, fmt.Errorf("NFS server not ready: %s", message)
	}

	// NFS server is ready
	return ctrl.Result{}, nil
}

// GenerateNfsServerName generates a unique NFS server name
func (n *NfsServerManager) GenerateNfsServerName(baseName string) string {
	return "nfs-" + baseName + "-" + n.RandomGen.RandString(6)
}

// UpdateNfsServerURL updates the NFS server URL in the volume spec
func (n *NfsServerManager) UpdateNfsServerURL(volumeObj types.VolumeObject, namespace string) {
	spec := volumeObj.GetVolumeSpec()
	if spec.NfsServer != nil && spec.NfsServer.Name != "" {
		spec.NfsServer.URL = spec.NfsServer.Name + "." + namespace + ".svc.cluster.local"
	}
}
