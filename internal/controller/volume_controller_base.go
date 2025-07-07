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
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	nfsv1alpha1 "github.com/sharedvolume/nfs-server-controller/api/v1alpha1"
	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

const (
	// NfsServerGeneratingAnnotation marks when NfsServer generation is in progress
	NfsServerGeneratingAnnotation = "shared-volume.io/nfsserver-generating"
)

// VolumeObject represents a common interface for volume-like objects
type VolumeObject interface {
	client.Object
	// GetVolumeSpec returns the common volume specification
	GetVolumeSpec() *svv1alpha1.VolumeSpecBase
	// GetPhase returns the current phase
	GetPhase() string
	// SetPhase sets the current phase
	SetPhase(phase string)
	// GetMessage returns the current message
	GetMessage() string
	// SetMessage sets the current message
	SetMessage(message string)
	// GetNfsServerAddress returns the NFS server address
	GetNfsServerAddress() string
	// SetNfsServerAddress sets the NFS server address
	SetNfsServerAddress(address string)
	// GetPersistentVolumeClaimName returns the PVC name
	GetPersistentVolumeClaimName() string
	// SetPersistentVolumeClaimName sets the PVC name
	SetPersistentVolumeClaimName(name string)
	// GetPersistentVolumeName returns the PV name
	GetPersistentVolumeName() string
	// SetPersistentVolumeName sets the PV name
	SetPersistentVolumeName(name string)
	// GetServiceName returns the service name
	GetServiceName() string
	// SetServiceName sets the service name
	SetServiceName(name string)
}

// VolumeControllerBase contains the shared implementation for volume controllers
type VolumeControllerBase struct {
	client.Client
	Scheme *runtime.Scheme
	// ControllerNamespace is the namespace where controller resources will be created
	ControllerNamespace string
	// SyncController handles sync operations
	SyncController *SyncController
}

// NewVolumeControllerBase creates a new VolumeControllerBase instance
func NewVolumeControllerBase(client client.Client, scheme *runtime.Scheme, controllerNamespace string, syncController *SyncController) *VolumeControllerBase {
	return &VolumeControllerBase{
		Client:              client,
		Scheme:              scheme,
		ControllerNamespace: controllerNamespace,
		SyncController:      syncController,
	}
}

// FillAndValidateSpec fills defaults and validates the volume spec.
func (r *VolumeControllerBase) FillAndValidateSpec(volumeObj VolumeObject, generateNfsServer bool, namespace string) error {
	spec := volumeObj.GetVolumeSpec()
	if spec.ResourceNamespace == "" {
		spec.ResourceNamespace = namespace
	}
	if spec.MountPath == "" {
		return errors.New("mountPath is required in volume spec")
	}
	if spec.SyncInterval == "" {
		spec.SyncInterval = "60s"
	}
	if spec.SyncTimeout == "" {
		spec.SyncTimeout = "120s"
	}
	if spec.Storage == nil {
		return errors.New("storage is required in volume spec")
	}
	if spec.Storage.Capacity == "" {
		return errors.New("storage.capacity is required in volume spec")
	}
	if spec.Storage.AccessMode == "" {
		spec.Storage.AccessMode = "ReadOnly"
	} else if spec.Storage.AccessMode != "ReadWrite" && spec.Storage.AccessMode != "ReadOnly" {
		return errors.New("storage.accessMode must be either ReadWrite or ReadOnly in volume spec")
	}
	if generateNfsServer && spec.StorageClassName == "" {
		return errors.New("storageClassName is required if nfsServer is not defined")
	}
	// Ensure NfsServer has a Path if it's defined, defaulting to "/"
	// The actual NfsServer and PV will use this exact path for the share
	if !generateNfsServer && spec.NfsServer != nil {
		if spec.NfsServer.Path == "" {
			spec.NfsServer.Path = "/"
		}
	}
	return nil
}

// CreateAndOwnNfsServer creates an NfsServer resource and sets ownership
func (r *VolumeControllerBase) CreateAndOwnNfsServer(ctx context.Context, volumeObj VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Check if NfsServer already exists to prevent duplicates
	existingNfsServer := &nfsv1alpha1.NfsServer{}
	err := r.Client.Get(ctx, client.ObjectKey{
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

	// Use the path from the spec for NfsServer path
	// We'll use spec.NfsServer.Path for the actual share path as requested

	nfsServer := &nfsv1alpha1.NfsServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      spec.NfsServer.Name,
			Namespace: namespace, // Use passed namespace parameter
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
	return r.Client.Create(ctx, nfsServer)
}

// RandString generates a random alphanumeric string of given length.
func (r *VolumeControllerBase) RandString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
	rand.Seed(time.Now().UnixNano())
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// GenerateAndCreateNfsServer creates an NFS server for the VolumeObject with default configuration
func (r *VolumeControllerBase) GenerateAndCreateNfsServer(ctx context.Context, volumeObj VolumeObject, resourcePrefix string, namespace string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Generate reference value if not set
	if spec.ReferenceValue == "" {
		spec.ReferenceValue = resourcePrefix + "-" + r.RandString(12)
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
	if err := r.Update(ctx, volumeObj); err != nil {
		log.Error(err, "Failed to update VolumeObject with generated NFS server spec")
		return ctrl.Result{}, err
	}

	// Create the NFS server
	if err := r.CreateAndOwnNfsServer(ctx, volumeObj, namespace); err != nil {
		log.Error(err, "Failed to create NFS server")
		return ctrl.Result{}, err
	}

	// Requeue to allow the NFS server to be created and become ready
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

// EnsureControllerNamespace ensures that the controller namespace exists
func (r *VolumeControllerBase) EnsureControllerNamespace(ctx context.Context) error {
	log := logf.FromContext(ctx)

	// Check if the namespace exists
	ns := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: r.ControllerNamespace}, ns)

	// If the namespace exists, return nil
	if err == nil {
		return nil
	}

	// If the error is not NotFound, return the error
	if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to check if controller namespace exists")
		return err
	}

	// Create the namespace
	log.Info("Creating controller namespace", "namespace", r.ControllerNamespace)
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.ControllerNamespace,
		},
	}

	return r.Create(ctx, ns)
}

// ReconcileNfsServer handles the NfsServer lifecycle management
func (r *VolumeControllerBase) ReconcileNfsServer(ctx context.Context, volumeObj VolumeObject, generateNfsServer bool, namespace string) (ctrl.Result, error) {
	// If we need to generate an NfsServer, this should be handled by the specific controller
	if generateNfsServer {
		return ctrl.Result{}, fmt.Errorf("NFS server generation should be handled by the specific volume controller")
	}

	// Check if NfsServer exists and is ready
	spec := volumeObj.GetVolumeSpec()
	if spec.NfsServer != nil && spec.NfsServer.Name != "" {
		return r.CheckAndUpdateNfsServerStatus(ctx, volumeObj, namespace)
	}

	return ctrl.Result{}, nil
}

// CheckAndUpdateNfsServerStatus checks NfsServer status and updates volume status accordingly
func (r *VolumeControllerBase) CheckAndUpdateNfsServerStatus(ctx context.Context, volumeObj VolumeObject, namespace string) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	ready, phase, message, err := r.CheckNfsServerStatus(ctx, volumeObj, namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update the status fields
	statusChanged := r.UpdateVolumeStatus(volumeObj, ready, phase, message)

	// Update status if changed (this will be handled by the calling controller for now)
	if statusChanged {
		log.Info("NFS server status changed",
			"ready", ready,
			"phase", phase,
			"message", message)
		// Status update will be handled by the calling controller
	} else {
		log.V(1).Info("NFS server status unchanged, skipping status update")
	}

	// If NFS server not ready yet, requeue
	if !ready {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// NfsServer is ready, proceeding to resource reconciliation
	log.Info("NfsServer is ready, proceeding to resource reconciliation")
	return ctrl.Result{}, nil
}

// CheckNfsServerStatus checks if the NfsServer exists and is ready
func (r *VolumeControllerBase) CheckNfsServerStatus(ctx context.Context, volumeObj VolumeObject, namespace string) (bool, string, string, error) {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	nfsServer := &nfsv1alpha1.NfsServer{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      spec.NfsServer.Name,
		Namespace: namespace,
	}, nfsServer)

	if client.IgnoreNotFound(err) != nil {
		log.Error(err, "Error getting NfsServer")
		return false, "Error", "Failed to get NfsServer: " + err.Error(), err
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

	// NFS server is ready - don't set the phase here, let checkResourceReadiness determine the final status
	return true, "", "NfsServer is ready", nil
}

// UpdateVolumeStatus updates the status fields and returns true if any field was changed
func (r *VolumeControllerBase) UpdateVolumeStatus(volumeObj VolumeObject, nfsReady bool, phase string, message string) bool {
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

	// Note: Ready status will be updated by checkResourceReadiness based on overall readiness
	// Don't update Ready status here to avoid conflicts

	return statusChanged
}

// UpdateVolumeStatusSimple updates basic status fields (phase and message) and returns true if any field was changed
func (r *VolumeControllerBase) UpdateVolumeStatusSimple(volumeObj VolumeObject, phase string, message string) bool {
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

// RemoveFinalizerWithRetry removes a finalizer from any client.Object with retry and conflict handling
func (r *VolumeControllerBase) RemoveFinalizerWithRetry(ctx context.Context, obj client.Object, finalizerName string) error {
	log := logf.FromContext(ctx)

	// Maximum number of retries
	maxRetries := 3
	retryCount := 0

	for {
		// Get the latest version of the object before updating to prevent conflicts
		latestObj := obj.DeepCopyObject().(client.Object)
		if err := r.Get(ctx, client.ObjectKeyFromObject(obj), latestObj); err != nil {
			if apierrors.IsNotFound(err) {
				// Object already deleted, nothing to do
				log.Info("Object already deleted, no need to remove finalizer", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
				return nil
			}
			log.Error(err, "Failed to get latest object before finalizer removal", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
			return err
		}

		// Check if the finalizer is still present
		if !ContainsString(latestObj.GetFinalizers(), finalizerName) {
			log.Info("Finalizer already removed", "finalizer", finalizerName, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
			// Update the original object reference with the latest version
			obj.SetResourceVersion(latestObj.GetResourceVersion())
			obj.SetFinalizers(latestObj.GetFinalizers())
			return nil
		}

		// Remove the finalizer from the latest version
		finalizers := RemoveString(latestObj.GetFinalizers(), finalizerName)
		latestObj.SetFinalizers(finalizers)

		// Update the object
		if err := r.Update(ctx, latestObj); err != nil {
			if apierrors.IsConflict(err) && retryCount < maxRetries {
				retryCount++
				log.Info("Conflict detected while removing finalizer, retrying", "retryCount", retryCount, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
				time.Sleep(time.Millisecond * 100 * time.Duration(retryCount)) // Backoff with each retry
				continue
			}
			if apierrors.IsNotFound(err) {
				// Object was deleted during our retry, which is fine
				log.Info("Object was deleted during finalizer removal", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
				return nil
			}
			log.Error(err, "Failed to remove finalizer after retries", "finalizer", finalizerName, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
			return err
		}

		// Success - update our reference with the latest version
		obj.SetResourceVersion(latestObj.GetResourceVersion())
		obj.SetFinalizers(latestObj.GetFinalizers())
		log.Info("Successfully removed finalizer", "finalizer", finalizerName, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
		return nil
	}
}

// UpdateStatusWithRetry updates the status of any client.Object with retry and conflict handling
func (r *VolumeControllerBase) UpdateStatusWithRetry(ctx context.Context, obj client.Object, updateFn func(client.Object)) error {
	log := logf.FromContext(ctx)

	// Maximum number of retries
	maxRetries := 3
	retryCount := 0

	for {
		// Get the latest version of the object before updating to prevent conflicts
		latestObj := obj.DeepCopyObject().(client.Object)
		if err := r.Get(ctx, client.ObjectKeyFromObject(obj), latestObj); err != nil {
			log.Error(err, "Failed to get latest object before status update", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
			return err
		}

		// Apply the status update function to the latest object
		updateFn(latestObj)

		// Update the status on the latest version of the object
		if err := r.Status().Update(ctx, latestObj); err != nil {
			if apierrors.IsConflict(err) && retryCount < maxRetries {
				retryCount++
				log.Info("Conflict detected while updating status, retrying", "retryCount", retryCount, "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
				time.Sleep(time.Millisecond * 100 * time.Duration(retryCount)) // Backoff with each retry
				continue
			}
			log.Error(err, "Failed to update object status after retries", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
			return err
		}

		// Success - update our reference with the latest version
		obj.SetResourceVersion(latestObj.GetResourceVersion())
		// Note: We don't copy the entire object here as the caller may need to preserve other changes
		// The caller should handle copying any needed fields from latestObj if required
		log.V(1).Info("Successfully updated object status", "kind", obj.GetObjectKind().GroupVersionKind().Kind, "name", obj.GetName())
		return nil
	}
}

// ReconcileRequiredResources creates or updates the PV, PVC, ReplicaSet, and Service resources
func (r *VolumeControllerBase) ReconcileRequiredResources(ctx context.Context, volumeObj VolumeObject, namespace string) error {
	spec := volumeObj.GetVolumeSpec()

	// If referenceValue is not set, return an error - this should be handled by the specific controller
	if spec.ReferenceValue == "" {
		return fmt.Errorf("referenceValue must be set before calling ReconcileRequiredResources")
	}

	// Create PV
	if err := r.ReconcilePersistentVolume(ctx, volumeObj, namespace); err != nil {
		return err
	}

	// Create PVC
	if err := r.ReconcilePersistentVolumeClaim(ctx, volumeObj, namespace); err != nil {
		return err
	}

	// Create ReplicaSet
	if err := r.ReconcileReplicaSet(ctx, volumeObj, namespace); err != nil {
		return err
	}

	// Create Service
	if err := r.ReconcileService(ctx, volumeObj, namespace); err != nil {
		return err
	}

	return nil
}

// ReconcilePersistentVolume creates or updates the PersistentVolume
func (r *VolumeControllerBase) ReconcilePersistentVolume(ctx context.Context, volumeObj VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Define PV name
	pvName := fmt.Sprintf("pv-%s", spec.ReferenceValue)

	// Check if PV already exists
	existingPV := &corev1.PersistentVolume{}
	err := r.Get(ctx, client.ObjectKey{Name: pvName}, existingPV)
	if err == nil {
		// PV already exists, update status if needed
		if volumeObj.GetPersistentVolumeName() != pvName {
			// Status update will be handled by the calling controller
			log.Info("PV already exists", "name", pvName)
		}
		return nil
	}

	// Get storage capacity as a resource quantity
	storageQuantity, err := resource.ParseQuantity(spec.Storage.Capacity)
	if err != nil {
		log.Error(err, "Failed to parse storage capacity", "capacity", spec.Storage.Capacity)
		return err
	}

	err = r.Get(ctx, client.ObjectKey{Name: pvName}, existingPV)
	if err == nil {
		// PV already exists, update status if needed
		if volumeObj.GetPersistentVolumeName() != pvName {
			// Status update will be handled by the calling controller
			log.Info("PV already exists", "name", pvName)
		}
		return nil
	}

	// Define the PV
	// Use the path from the spec for the share
	// This needs to match the actual path used in the NFS server
	nfsSharePath := spec.NfsServer.Path

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "shared-volume-controller",
				"shared-volume.io/reference":   spec.ReferenceValue,
				"shared-volume.io/owner":       fmt.Sprintf("%s.%s", volumeObj.GetName(), namespace),
			},
			Annotations: map[string]string{
				"pv.kubernetes.io/provisioned-by": "nfs.csi.k8s.io",
			},
		},
		Spec: corev1.PersistentVolumeSpec{
			Capacity: corev1.ResourceList{
				corev1.ResourceStorage: storageQuantity,
			},
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
			MountOptions: []string{
				"nfsvers=4.1",
			},
			StorageClassName: "",
			PersistentVolumeSource: corev1.PersistentVolumeSource{
				CSI: &corev1.CSIPersistentVolumeSource{
					Driver: "nfs.csi.k8s.io",
					VolumeHandle: fmt.Sprintf("%s%s##",
						spec.NfsServer.URL,
						nfsSharePath),
					VolumeAttributes: map[string]string{
						"server": spec.NfsServer.URL,
						"share":  nfsSharePath,
					},
				},
			},
		},
	}

	// Create the PV
	if err := r.Create(ctx, pv); err != nil {
		log.Error(err, "Failed to create PersistentVolume", "name", pvName)
		return err
	}

	log.Info("Created PersistentVolume", "name", pvName)
	return nil
}

// ReconcilePersistentVolumeClaim creates or updates the PersistentVolumeClaim
func (r *VolumeControllerBase) ReconcilePersistentVolumeClaim(ctx context.Context, volumeObj VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Define PVC name
	pvcName := spec.ReferenceValue
	pvName := fmt.Sprintf("pv-%s", spec.ReferenceValue)

	// Check if PVC already exists
	existingPVC := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      pvcName,
		Namespace: namespace,
	}, existingPVC)

	if err == nil {
		// PVC already exists, update status if needed
		if volumeObj.GetPersistentVolumeClaimName() != pvcName {
			// Status update will be handled by the calling controller
			log.Info("PVC already exists", "name", pvcName)
		}
		return nil
	}

	// Get storage capacity as a resource quantity
	storageQuantity, err := resource.ParseQuantity(spec.Storage.Capacity)
	if err != nil {
		log.Error(err, "Failed to parse storage capacity", "capacity", spec.Storage.Capacity)
		return err
	}

	// Define the PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
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
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageQuantity,
				},
			},
			VolumeName:       pvName,
			StorageClassName: pointer.String(""), // Set to empty string explicitly
		},
	}

	// Create the PVC
	if err := r.Create(ctx, pvc); err != nil {
		log.Error(err, "Failed to create PersistentVolumeClaim", "name", pvcName)
		return err
	}

	log.Info("Created PersistentVolumeClaim", "name", pvcName)
	return nil
}

// ReconcileReplicaSet creates or updates the ReplicaSet
func (r *VolumeControllerBase) ReconcileReplicaSet(ctx context.Context, volumeObj VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Define ReplicaSet name
	replicaSetName := spec.ReferenceValue
	pvcName := spec.ReferenceValue

	// Check if ReplicaSet already exists
	existingRS := &appsv1.ReplicaSet{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      replicaSetName,
		Namespace: namespace,
	}, existingRS)

	if err == nil {
		// ReplicaSet already exists, check if it has the correct configuration
		needsUpdate := false

		// Check if container port is correct (should be 8080)
		if len(existingRS.Spec.Template.Spec.Containers) > 0 &&
			len(existingRS.Spec.Template.Spec.Containers[0].Ports) > 0 {
			currentPort := existingRS.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort
			if currentPort != 8080 {
				log.Info("ReplicaSet has incorrect port configuration, will recreate",
					"name", replicaSetName,
					"currentPort", currentPort,
					"expectedPort", 8080)
				needsUpdate = true
			}
		}

		if needsUpdate {
			// Delete the existing ReplicaSet so it can be recreated with correct config
			log.Info("Deleting ReplicaSet with incorrect configuration", "name", replicaSetName)
			if err := r.Delete(ctx, existingRS); err != nil {
				log.Error(err, "Failed to delete ReplicaSet for update", "name", replicaSetName)
				return err
			}
			// Continue to create a new one below
		} else {
			// ReplicaSet exists and has correct configuration
			log.Info("ReplicaSet already exists with correct configuration", "name", replicaSetName)
			return nil
		}
	}

	// Define the ReplicaSet
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
			Template: corev1.PodTemplateSpec{
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
			},
		},
	}

	// Create the ReplicaSet
	if err := r.Create(ctx, rs); err != nil {
		log.Error(err, "Failed to create ReplicaSet", "name", replicaSetName)
		return err
	}

	log.Info("Created ReplicaSet", "name", replicaSetName, "referenceID", spec.ReferenceValue)
	return nil
}

// ReconcileService creates or updates the Service
func (r *VolumeControllerBase) ReconcileService(ctx context.Context, volumeObj VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Define Service name
	serviceName := fmt.Sprintf("%s", spec.ReferenceValue)
	replicaSetName := spec.ReferenceValue

	// Check if Service already exists
	existingSvc := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      serviceName,
		Namespace: namespace,
	}, existingSvc)

	if err == nil {
		// Service already exists, check if it has the correct configuration
		needsUpdate := false

		// Check if service port is correct (should be 8080)
		if len(existingSvc.Spec.Ports) > 0 {
			currentPort := existingSvc.Spec.Ports[0].Port
			if currentPort != 8080 {
				log.Info("Service has incorrect port configuration, will recreate",
					"name", serviceName,
					"currentPort", currentPort,
					"expectedPort", 8080)
				needsUpdate = true
			}
		}

		if needsUpdate {
			// Delete the existing Service so it can be recreated with correct config
			log.Info("Deleting Service with incorrect configuration", "name", serviceName)
			if err := r.Delete(ctx, existingSvc); err != nil {
				log.Error(err, "Failed to delete Service for update", "name", serviceName)
				return err
			}
			// Continue to create a new one below
		} else {
			// Service exists and has correct configuration
			log.Info("Service already exists with correct configuration", "name", serviceName)
			return nil
		}
	}

	// Define the Service
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
	if err := r.Create(ctx, svc); err != nil {
		log.Error(err, "Failed to create Service", "name", serviceName)
		return err
	}

	log.Info("Created Service", "name", serviceName)
	return nil
}

// Cleanup and finalizer management functions

// CleanupResources removes all resources associated with a volume
func (b *VolumeControllerBase) CleanupResources(ctx context.Context, volume VolumeObject) error {
	log := logf.FromContext(ctx)

	// Stop sync operations first - this should be handled by the specific controller
	// since sync operations are volume-type specific
	// The specific controller should call the appropriate sync stop method

	spec := volume.GetVolumeSpec()
	if spec.ReferenceValue == "" {
		return nil
	}

	referenceValue := spec.ReferenceValue

	log.Info("Starting comprehensive cleanup for volume", "name", volume.GetName(), "referenceValue", referenceValue)

	// 1. Find and force delete ALL pods that use this volume (not just ReplicaSet pods)
	b.CleanupAllPodsUsingVolume(ctx, volume)

	// 2. Delete ReplicaSet and any remaining pods
	b.CleanupReplicaSet(ctx, volume, referenceValue)

	// 3. Wait for pods to be fully terminated before deleting PVCs
	b.WaitForPodsToTerminate(ctx, volume, referenceValue)

	// 4. Delete Service
	b.CleanupService(ctx, volume, referenceValue)

	// 5. Delete all PVCs related to this volume (both main and namespace-specific ones)
	b.CleanupAllPVCs(ctx, volume, referenceValue)

	// 6. Delete all PVs related to this volume (both main and namespace-specific ones)
	b.CleanupAllPVs(ctx, volume, referenceValue)

	// 7. Delete NFS Server if it was generated
	b.CleanupNFSServer(ctx, volume)

	log.Info("Completed comprehensive cleanup for volume", "name", volume.GetName())
	return nil
}

// CleanupAllPodsUsingVolume finds and force deletes all pods that use this volume
func (b *VolumeControllerBase) CleanupAllPodsUsingVolume(ctx context.Context, volume VolumeObject) {
	log := logf.FromContext(ctx)

	// Search across all namespaces for pods using this volume
	podList := &corev1.PodList{}
	if err := b.List(ctx, podList); err != nil {
		log.Error(err, "Failed to list pods for volume cleanup")
		return
	}

	volumeName := volume.GetName()
	volumeNamespace := volume.GetNamespace()

	for _, pod := range podList.Items {
		// Check if pod has volume annotations
		if pod.Annotations != nil {
			for key, value := range pod.Annotations {
				if strings.HasPrefix(key, "sharedvolume.sv/") && value == "true" {
					// Extract the volume reference from the annotation key
					refPart := strings.TrimPrefix(key, "sharedvolume.sv/")

					var namespace, name string
					if strings.Contains(refPart, "__") {
						parts := strings.SplitN(refPart, "__", 2)
						namespace = parts[0]
						name = parts[1]
					} else {
						namespace = pod.Namespace
						name = refPart
					}

					// Check if this pod references our volume
					if name == volumeName && namespace == volumeNamespace {
						log.Info("Found pod using volume, force deleting",
							"pod", pod.Name,
							"podNamespace", pod.Namespace,
							"volume", volumeName)

						if err := b.ForceDeletePod(ctx, &pod); err != nil {
							log.Error(err, "Failed to force delete pod using volume",
								"pod", pod.Name, "namespace", pod.Namespace)
						}
					}
				}
			}
		}
	}
}

// CleanupReplicaSet deletes the ReplicaSet and any remaining pods
func (b *VolumeControllerBase) CleanupReplicaSet(ctx context.Context, volume VolumeObject, referenceValue string) {
	log := logf.FromContext(ctx)

	rs := &appsv1.ReplicaSet{}
	if err := b.Get(ctx, client.ObjectKey{
		Name:      referenceValue,
		Namespace: volume.GetNamespace(),
	}, rs); err == nil {
		// First, explicitly delete all pods owned by this ReplicaSet
		podList := &corev1.PodList{}
		if err := b.List(ctx, podList, client.InNamespace(volume.GetNamespace()), client.MatchingLabels{"app": referenceValue}); err == nil {
			for _, pod := range podList.Items {
				log.Info("Force deleting ReplicaSet pod", "name", pod.Name, "namespace", pod.Namespace)
				if err := b.ForceDeletePod(ctx, &pod); err != nil {
					log.Error(err, "Failed to force delete ReplicaSet pod", "name", pod.Name)
				}
			}
		}

		// Then delete the ReplicaSet
		log.Info("Deleting ReplicaSet", "name", referenceValue, "namespace", volume.GetNamespace())
		if err := b.Delete(ctx, rs); err != nil {
			log.Error(err, "Failed to delete ReplicaSet", "name", referenceValue)
		}
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get ReplicaSet", "name", referenceValue)
	}
}

// CleanupService deletes the service
func (b *VolumeControllerBase) CleanupService(ctx context.Context, volume VolumeObject, referenceValue string) {
	log := logf.FromContext(ctx)

	svcName := fmt.Sprintf("%s", referenceValue)
	svc := &corev1.Service{}
	if err := b.Get(ctx, client.ObjectKey{
		Name:      svcName,
		Namespace: volume.GetNamespace(),
	}, svc); err == nil {
		log.Info("Deleting Service", "name", svcName, "namespace", volume.GetNamespace())
		if err := b.Delete(ctx, svc); err != nil {
			log.Error(err, "Failed to delete Service", "name", svcName)
		}
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get Service", "name", svcName)
	}
}

// CleanupAllPVCs deletes all PVCs related to this volume
func (b *VolumeControllerBase) CleanupAllPVCs(ctx context.Context, volume VolumeObject, referenceValue string) {
	log := logf.FromContext(ctx)

	// 1. Delete the main PVC
	pvc := &corev1.PersistentVolumeClaim{}
	if err := b.Get(ctx, client.ObjectKey{
		Name:      referenceValue,
		Namespace: volume.GetNamespace(),
	}, pvc); err == nil {
		log.Info("Force deleting main PVC", "name", referenceValue, "namespace", volume.GetNamespace())
		if err := b.ForceDeletePVC(ctx, pvc); err != nil {
			log.Error(err, "Failed to force delete main PVC", "name", referenceValue)
		}
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get main PVC", "name", referenceValue)
	}

	// 2. Delete namespace-specific PVCs (pattern: referenceValue-namespace)
	pvcList := &corev1.PersistentVolumeClaimList{}
	if err := b.List(ctx, pvcList); err == nil {
		for _, pvcItem := range pvcList.Items {
			// Check if PVC name follows the pattern: referenceValue-namespace
			if strings.HasPrefix(pvcItem.Name, referenceValue+"-") {
				log.Info("Force deleting namespace-specific PVC", "name", pvcItem.Name, "namespace", pvcItem.Namespace)
				if err := b.ForceDeletePVC(ctx, &pvcItem); err != nil {
					log.Error(err, "Failed to force delete namespace-specific PVC", "name", pvcItem.Name)
				}
			}
		}
	} else {
		log.Error(err, "Failed to list PVCs for cleanup")
	}
}

// CleanupAllPVs deletes all PVs related to this volume
func (b *VolumeControllerBase) CleanupAllPVs(ctx context.Context, volume VolumeObject, referenceValue string) {
	log := logf.FromContext(ctx)

	// 1. Delete the main PV
	pvName := fmt.Sprintf("pv-%s", referenceValue)
	pv := &corev1.PersistentVolume{}
	if err := b.Get(ctx, client.ObjectKey{Name: pvName}, pv); err == nil {
		log.Info("Force deleting main PV", "name", pvName)
		if err := b.ForceDeletePV(ctx, pv); err != nil {
			log.Error(err, "Failed to force delete main PV", "name", pvName)
		}
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get main PV", "name", pvName)
	}

	// 2. Delete namespace-specific PVs (pattern: referenceValue-namespace)
	pvList := &corev1.PersistentVolumeList{}
	if err := b.List(ctx, pvList); err == nil {
		for _, pvItem := range pvList.Items {
			// Check if PV name follows the pattern: referenceValue-namespace
			if strings.HasPrefix(pvItem.Name, referenceValue+"-") {
				log.Info("Force deleting namespace-specific PV", "name", pvItem.Name)
				if err := b.ForceDeletePV(ctx, &pvItem); err != nil {
					log.Error(err, "Failed to force delete namespace-specific PV", "name", pvItem.Name)
				}
			}
		}
	} else {
		log.Error(err, "Failed to list PVs for cleanup")
	}
}

// CleanupNFSServer deletes the NFS server if it was generated
func (b *VolumeControllerBase) CleanupNFSServer(ctx context.Context, volume VolumeObject) {
	log := logf.FromContext(ctx)

	spec := volume.GetVolumeSpec()
	if spec.NfsServer != nil && spec.NfsServer.Name != "" {
		nfsServer := &nfsv1alpha1.NfsServer{}
		if err := b.Get(ctx, client.ObjectKey{
			Name:      spec.NfsServer.Name,
			Namespace: volume.GetNamespace(),
		}, nfsServer); err == nil {
			log.Info("Deleting NFS Server", "name", spec.NfsServer.Name, "namespace", volume.GetNamespace())
			if err := b.Delete(ctx, nfsServer); err != nil {
				log.Error(err, "Failed to delete NfsServer", "name", spec.NfsServer.Name)
			}
		} else if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get NFS Server", "name", spec.NfsServer.Name)
		}
	}
}

// ForceDeletePVC attempts to delete a PVC and removes finalizers if deletion is stuck
func (b *VolumeControllerBase) ForceDeletePVC(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
	log := logf.FromContext(ctx)

	log.Info("Deleting PVC", "name", pvc.Name, "namespace", pvc.Namespace)

	// First check if PVC has problematic finalizers and remove them proactively
	if len(pvc.Finalizers) > 0 {
		log.Info("PVC has finalizers, removing them before deletion to prevent getting stuck",
			"name", pvc.Name,
			"namespace", pvc.Namespace,
			"finalizers", pvc.Finalizers)

		if err := b.RemovePVCFinalizersWithRetry(ctx, pvc); err != nil {
			log.Error(err, "Failed to proactively remove PVC finalizers", "name", pvc.Name)
			// Continue with deletion attempt even if finalizer removal failed
		}
	}

	// Now try normal deletion
	if err := b.Delete(ctx, pvc); err != nil {
		log.Error(err, "Failed to delete PVC normally", "name", pvc.Name)
		return err
	}

	// Check if PVC is stuck in terminating state due to finalizers
	// Give it a moment to delete normally first
	var updatedPVC corev1.PersistentVolumeClaim
	if err := b.Get(ctx, client.ObjectKeyFromObject(pvc), &updatedPVC); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PVC successfully deleted", "name", pvc.Name, "namespace", pvc.Namespace)
			return nil
		}
		return err
	}

	// PVC still exists, check if it's terminating and has finalizers
	if updatedPVC.DeletionTimestamp != nil && len(updatedPVC.Finalizers) > 0 {
		log.Info("PVC is stuck terminating, removing finalizers",
			"name", pvc.Name,
			"namespace", pvc.Namespace,
			"finalizers", updatedPVC.Finalizers)

		// Remove all finalizers to force deletion with retry
		if err := b.RemovePVCFinalizersWithRetry(ctx, &updatedPVC); err != nil {
			log.Error(err, "Failed to remove finalizers from PVC", "name", pvc.Name)
			return err
		}
	}

	return nil
}

// ForceDeletePV attempts to delete a PV and removes finalizers if deletion is stuck
func (b *VolumeControllerBase) ForceDeletePV(ctx context.Context, pv *corev1.PersistentVolume) error {
	log := logf.FromContext(ctx)

	log.Info("Deleting PV", "name", pv.Name)

	// First check if PV has CSI or other problematic finalizers and remove them proactively
	if len(pv.Finalizers) > 0 {
		log.Info("PV has finalizers, removing them before deletion to prevent getting stuck",
			"name", pv.Name,
			"finalizers", pv.Finalizers)

		if err := b.RemovePVFinalizersWithRetry(ctx, pv); err != nil {
			log.Error(err, "Failed to proactively remove PV finalizers", "name", pv.Name)
			// Continue with deletion attempt even if finalizer removal failed
		}
	}

	// Now try normal deletion
	if err := b.Delete(ctx, pv); err != nil {
		log.Error(err, "Failed to delete PV normally", "name", pv.Name)
		return err
	}

	// Check if PV is stuck in terminating state due to finalizers
	// Give it a moment to delete normally first
	var updatedPV corev1.PersistentVolume
	if err := b.Get(ctx, client.ObjectKeyFromObject(pv), &updatedPV); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("PV successfully deleted", "name", pv.Name)
			return nil
		}
		return err
	}

	// PV still exists, check if it's terminating and has finalizers
	if updatedPV.DeletionTimestamp != nil && len(updatedPV.Finalizers) > 0 {
		log.Info("PV is stuck terminating, removing finalizers",
			"name", pv.Name,
			"finalizers", updatedPV.Finalizers)

		// Remove all finalizers to force deletion with retry
		if err := b.RemovePVFinalizersWithRetry(ctx, &updatedPV); err != nil {
			log.Error(err, "Failed to remove finalizers from PV", "name", pv.Name)
			return err
		}
	}

	return nil
}

// RemovePVFinalizersWithRetry removes finalizers from a PV with retry logic to handle conflicts
func (b *VolumeControllerBase) RemovePVFinalizersWithRetry(ctx context.Context, pv *corev1.PersistentVolume) error {
	log := logf.FromContext(ctx)

	maxRetries := 3
	retryCount := 0

	for {
		// Get the latest version of the PV
		var latestPV corev1.PersistentVolume
		if err := b.Get(ctx, client.ObjectKey{Name: pv.Name}, &latestPV); err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("PV already deleted during finalizer removal", "name", pv.Name)
				return nil
			}
			return err
		}

		// Remove all finalizers
		latestPV.Finalizers = []string{}

		// Try to update
		if err := b.Update(ctx, &latestPV); err != nil {
			if apierrors.IsConflict(err) && retryCount < maxRetries {
				retryCount++
				log.Info("Conflict updating PV finalizers, retrying",
					"name", pv.Name,
					"retry", retryCount,
					"maxRetries", maxRetries)
				continue
			}
			return err
		}

		log.Info("Successfully removed finalizers from PV", "name", pv.Name)
		return nil
	}
}

// RemovePVCFinalizersWithRetry removes finalizers from a PVC with retry logic to handle conflicts
func (b *VolumeControllerBase) RemovePVCFinalizersWithRetry(ctx context.Context, pvc *corev1.PersistentVolumeClaim) error {
	log := logf.FromContext(ctx)

	maxRetries := 3
	retryCount := 0

	for {
		// Get the latest version of the PVC
		var latestPVC corev1.PersistentVolumeClaim
		if err := b.Get(ctx, client.ObjectKey{
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
		}, &latestPVC); err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("PVC already deleted during finalizer removal", "name", pvc.Name, "namespace", pvc.Namespace)
				return nil
			}
			return err
		}

		// Remove all finalizers
		latestPVC.Finalizers = []string{}

		// Try to update
		if err := b.Update(ctx, &latestPVC); err != nil {
			if apierrors.IsConflict(err) && retryCount < maxRetries {
				retryCount++
				log.Info("Conflict updating PVC finalizers, retrying",
					"name", pvc.Name,
					"namespace", pvc.Namespace,
					"retry", retryCount,
					"maxRetries", maxRetries)
				continue
			}
			return err
		}

		log.Info("Successfully removed finalizers from PVC", "name", pvc.Name, "namespace", pvc.Namespace)
		return nil
	}
}

// ForceDeletePod attempts to delete a pod and removes all finalizers if deletion is stuck
func (b *VolumeControllerBase) ForceDeletePod(ctx context.Context, pod *corev1.Pod) error {
	log := logf.FromContext(ctx)

	log.Info("Deleting pod", "name", pod.Name, "namespace", pod.Namespace)

	// First try normal deletion
	if err := b.Delete(ctx, pod); err != nil {
		log.Error(err, "Failed to delete pod normally", "name", pod.Name)
		return err
	}

	// Check if pod is stuck in terminating state due to finalizers
	// Give it a moment to delete normally first
	var updatedPod corev1.Pod
	if err := b.Get(ctx, client.ObjectKeyFromObject(pod), &updatedPod); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Pod successfully deleted", "name", pod.Name, "namespace", pod.Namespace)
			return nil
		}
		return err
	}

	// Pod still exists, check if it's terminating and has finalizers
	if updatedPod.DeletionTimestamp != nil && len(updatedPod.Finalizers) > 0 {
		log.Info("Pod is stuck terminating, removing finalizers",
			"name", pod.Name,
			"namespace", pod.Namespace,
			"finalizers", updatedPod.Finalizers)

		// Remove all finalizers to force deletion
		updatedPod.Finalizers = []string{}
		if err := b.Update(ctx, &updatedPod); err != nil {
			log.Error(err, "Failed to remove finalizers from pod", "name", pod.Name)
			return err
		}
		log.Info("Successfully removed finalizers from pod", "name", pod.Name)
	}

	return nil
}

// IsReplicaSetReady checks if the ReplicaSet associated with the volume is ready
func (r *VolumeControllerBase) IsReplicaSetReady(ctx context.Context, volumeObj VolumeObject, namespace string) (bool, error) {
	log := logf.FromContext(ctx)

	// Get the ReplicaSet using the reference value
	spec := volumeObj.GetVolumeSpec()
	replicaSetName := spec.ReferenceValue
	rs := &appsv1.ReplicaSet{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      replicaSetName,
		Namespace: namespace,
	}, rs)

	if err != nil {
		log.Info("ReplicaSet not found", "name", replicaSetName, "error", err)
		return false, client.IgnoreNotFound(err)
	}

	// Check if ReplicaSet is ready (desired replicas == ready replicas)
	if rs.Spec.Replicas == nil {
		log.Info("ReplicaSet has nil replicas", "name", replicaSetName)
		return false, nil
	}

	desired := *rs.Spec.Replicas
	ready := rs.Status.ReadyReplicas
	isReady := ready == desired

	log.Info("ReplicaSet readiness check",
		"name", replicaSetName,
		"desired", desired,
		"ready", ready,
		"isReady", isReady)

	return isReady, nil
}

// IsPVCReady checks if the PVC associated with the volume is bound
func (r *VolumeControllerBase) IsPVCReady(ctx context.Context, volumeObj VolumeObject, namespace string) (bool, error) {
	log := logf.FromContext(ctx)

	spec := volumeObj.GetVolumeSpec()
	pvcName := spec.ReferenceValue
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      pvcName,
		Namespace: namespace,
	}, pvc)

	if err != nil {
		log.Error(err, "Failed to get PVC", "name", pvcName)
		return false, client.IgnoreNotFound(err)
	}

	// Log PVC status for debugging
	log.Info("PVC status check",
		"name", pvcName,
		"phase", pvc.Status.Phase,
		"capacity", pvc.Status.Capacity,
		"accessModes", pvc.Status.AccessModes)

	// Check if PVC is bound
	ready := pvc.Status.Phase == corev1.ClaimBound
	log.Info("PVC readiness result", "name", pvcName, "ready", ready)
	return ready, nil
}

// DeterminePhase determines the correct phase based on NFS and ReplicaSet readiness
func (r *VolumeControllerBase) DeterminePhase(nfsReady, replicaSetReady bool) string {
	if nfsReady && replicaSetReady {
		return "Ready"
	} else if nfsReady && !replicaSetReady {
		return "Preparing"
	} else {
		return "Pending"
	}
}

// UpdateReadinessStatus updates the volume status based on NFS and ReplicaSet readiness
func (r *VolumeControllerBase) UpdateReadinessStatus(ctx context.Context, volumeObj VolumeObject, nfsReady, replicaSetReady bool,
	onReady func(ctx context.Context, volumeObj VolumeObject) error,
	onNotReady func(ctx context.Context, volumeObj VolumeObject) error) error {
	log := logf.FromContext(ctx)

	phase := r.DeterminePhase(nfsReady, replicaSetReady)
	overallReady := phase == "Ready"
	spec := volumeObj.GetVolumeSpec()

	err := r.UpdateStatusWithRetry(ctx, volumeObj, func(obj client.Object) {
		if volumeObj, ok := obj.(VolumeObject); ok {
			volumeObj.SetPhase(phase)
			switch phase {
			case "Ready":
				volumeObj.SetMessage("Volume is ready for use")
			case "Preparing":
				volumeObj.SetMessage("NFS server is ready, waiting for ReplicaSet to be ready")
			case "Pending":
				if !nfsReady {
					volumeObj.SetMessage("Waiting for NFS server to be ready")
				} else {
					volumeObj.SetMessage("Waiting for resources to be ready")
				}
			}
		}
	})

	if err != nil {
		log.Error(err, "Failed to update volume status")
		return err
	}

	// Get the latest version to update our local reference
	latestObj := volumeObj.DeepCopyObject().(VolumeObject)
	if err := r.Get(ctx, client.ObjectKeyFromObject(volumeObj), latestObj); err != nil {
		return err
	}
	// Copy the latest version back to the original object
	// Note: This is a generic approach - specific controllers may need to handle this differently
	volumeObj.SetResourceVersion(latestObj.GetResourceVersion())
	volumeObj.SetPhase(latestObj.GetPhase())
	volumeObj.SetMessage(latestObj.GetMessage())

	if overallReady {
		log.Info("Volume resources are ready",
			"name", volumeObj.GetName(),
			"referenceID", spec.ReferenceValue,
			"phase", phase)

		// Call the ready callback if provided
		if onReady != nil {
			if err := onReady(ctx, volumeObj); err != nil {
				log.Error(err, "Failed to execute ready callback")
				// Don't return error, callbacks are not critical for resource readiness
			}
		}
	} else {
		log.Info("Volume is not fully ready",
			"name", volumeObj.GetName(),
			"referenceID", spec.ReferenceValue,
			"phase", phase,
			"nfsReady", nfsReady,
			"replicaSetReady", replicaSetReady)

		// Call the not ready callback if provided
		if onNotReady != nil {
			if err := onNotReady(ctx, volumeObj); err != nil {
				log.Error(err, "Failed to execute not ready callback")
				// Don't return error, callbacks are not critical
			}
		}
	}

	return nil
}

// ShouldUpdateReadinessStatus determines if a status update is needed based on the current status
func (r *VolumeControllerBase) ShouldUpdateReadinessStatus(volumeObj VolumeObject, nfsReady, replicaSetReady bool) bool {
	currentPhase := volumeObj.GetPhase()
	expectedPhase := r.DeterminePhase(nfsReady, replicaSetReady)
	return currentPhase != expectedPhase
}

// CheckResourceReadiness checks the readiness of all resources associated with a volume
func (r *VolumeControllerBase) CheckResourceReadiness(ctx context.Context, volumeObj VolumeObject, namespace string,
	onReady func(ctx context.Context, volumeObj VolumeObject) error,
	onNotReady func(ctx context.Context, volumeObj VolumeObject) error) error {
	log := logf.FromContext(ctx)

	// If reference value is not set, return
	spec := volumeObj.GetVolumeSpec()
	if spec.ReferenceValue == "" {
		return nil
	}

	// First check if NFS server is ready
	nfsReady, _, _, err := r.CheckNfsServerStatus(ctx, volumeObj, namespace)
	if err != nil {
		return err
	}

	if !nfsReady {
		log.Info("NFS server not ready, marking volume as not ready")
		if r.ShouldUpdateReadinessStatus(volumeObj, false, false) {
			if err := r.UpdateReadinessStatus(ctx, volumeObj, false, false, onReady, onNotReady); err != nil {
				return err
			}
		}
		return nil
	}

	// Check if PVC is bound
	pvcReady, err := r.IsPVCReady(ctx, volumeObj, namespace)
	if err != nil {
		return err
	}

	if !pvcReady {
		log.Info("PVC not ready yet", "name", spec.ReferenceValue)
		// Don't check ReplicaSet if PVC is not ready, just update status to not ready
		if r.ShouldUpdateReadinessStatus(volumeObj, nfsReady, false) {
			if err := r.UpdateReadinessStatus(ctx, volumeObj, nfsReady, false, onReady, onNotReady); err != nil {
				return err
			}
		}
		return nil
	}

	// Check if the ReplicaSet is ready
	replicaSetReady, err := r.IsReplicaSetReady(ctx, volumeObj, namespace)
	if err != nil {
		return err
	}

	// Overall readiness = NFS ready AND PVC ready AND ReplicaSet ready
	overallReady := nfsReady && pvcReady && replicaSetReady

	log.Info("Overall readiness check",
		"nfsReady", nfsReady,
		"pvcReady", pvcReady,
		"replicaSetReady", replicaSetReady,
		"overallReady", overallReady)

	// Determine if status update is needed
	if r.ShouldUpdateReadinessStatus(volumeObj, nfsReady, replicaSetReady) {
		log.Info("Resource readiness changed, updating status",
			"currentPhase", volumeObj.GetPhase(),
			"expectedPhase", r.DeterminePhase(nfsReady, replicaSetReady),
			"nfsReady", nfsReady,
			"replicaSetReady", replicaSetReady)
		// Update the status
		if err := r.UpdateReadinessStatus(ctx, volumeObj, nfsReady, replicaSetReady, onReady, onNotReady); err != nil {
			return err
		}
	} else {
		log.V(1).Info("Resource readiness unchanged, skipping status update",
			"phase", volumeObj.GetPhase(),
			"nfsReady", nfsReady,
			"replicaSetReady", replicaSetReady)
	}

	return nil
}

// ReconcileNfsServerComplete handles the complete NfsServer lifecycle management including
// generation, resource reconciliation, and readiness checking
func (r *VolumeControllerBase) ReconcileNfsServerComplete(ctx context.Context, volumeObj VolumeObject, namespace string, generateNfsServer bool, resourcePrefix string,
	onReady func(ctx context.Context, volumeObj VolumeObject) error,
	onNotReady func(ctx context.Context, volumeObj VolumeObject) error) (ctrl.Result, error) {

	// If we need to generate an NfsServer, handle it here
	if generateNfsServer {
		result, err := r.GenerateAndCreateNfsServer(ctx, volumeObj, resourcePrefix, namespace)
		if err != nil {
			return result, err
		}
		// If still in progress, return the result
		if result.RequeueAfter > 0 {
			return result, nil
		}
	}

	// Use the base controller for status checking and other NFS server management
	result, err := r.ReconcileNfsServer(ctx, volumeObj, false, namespace) // Always pass false since we handle generation above
	if err != nil {
		return result, err
	}

	// If NFS server is not ready yet, return the result from base
	if result.RequeueAfter > 0 {
		return result, nil
	}

	// Ensure referenceValue is set before reconciling resources
	spec := volumeObj.GetVolumeSpec()
	if spec.ReferenceValue == "" {
		spec.ReferenceValue = resourcePrefix + "-" + r.RandString(12)
		if err := r.Update(ctx, volumeObj); err != nil {
			return ctrl.Result{}, err
		}
		// Requeue to pick up the updated referenceValue
		return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
	}

	// NFS server is ready, continue with resource reconciliation
	if err := r.ReconcileRequiredResources(ctx, volumeObj, namespace); err != nil {
		return ctrl.Result{}, err
	}

	// Check if the resources are ready
	if err := r.CheckResourceReadiness(ctx, volumeObj, namespace, onReady, onNotReady); err != nil {
		return ctrl.Result{}, err
	}

	// Only requeue if the volume is not ready yet
	if volumeObj.GetPhase() != "Ready" {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// VolumeReconcileConfig holds the configuration for generic volume reconciliation
type VolumeReconcileConfig struct {
	VolumeObjFactory       func() VolumeObject
	APIVersion             string
	Kind                   string
	FinalizerName          string
	Namespace              string // The namespace to use for resource creation
	CreateReadyCallback    func() func(context.Context, VolumeObject) error
	CreateNotReadyCallback func() func(context.Context, VolumeObject) error
}

// ReconcileGeneric provides a complete generic reconciliation pattern for volume objects
func (r *VolumeControllerBase) ReconcileGeneric(
	ctx context.Context,
	req ctrl.Request,
	config VolumeReconcileConfig,
) (ctrl.Result, error) {

	// Create a new instance of the volume object
	volumeObj := config.VolumeObjFactory()

	// Fetch the volume instance
	if err := r.Get(ctx, req.NamespacedName, volumeObj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure APIVersion and Kind are set for owner references
	r.EnsureAPIVersionAndKind(volumeObj, config.APIVersion, config.Kind)

	// Create callbacks
	onReady := config.CreateReadyCallback()
	onNotReady := config.CreateNotReadyCallback()

	// Use the generic reconciliation pattern
	return r.ReconcileVolume(ctx, req, volumeObj, config.FinalizerName, config.Namespace, onReady, onNotReady)
}

// ReconcileVolume provides a generic reconciliation pattern for volume objects
func (r *VolumeControllerBase) ReconcileVolume(ctx context.Context, req ctrl.Request,
	volumeObj VolumeObject, finalizerName string, namespace string,
	onReady func(context.Context, VolumeObject) error,
	onNotReady func(context.Context, VolumeObject) error) (ctrl.Result, error) {

	log := logf.FromContext(ctx)

	// Ensure the controller namespace exists
	if err := r.EnsureControllerNamespace(ctx); err != nil {
		log.Error(err, "Failed to ensure controller namespace exists")
		return ctrl.Result{}, err
	}

	// Handle finalizer logic - returns true to continue, false to exit early
	shouldContinue, err := r.HandleFinalizerLogic(ctx, volumeObj, finalizerName)
	if !shouldContinue {
		return ctrl.Result{}, err
	}

	// Fill and validate spec
	generateNfsServer := volumeObj.GetVolumeSpec().NfsServer == nil
	if err := r.FillAndValidateSpec(volumeObj, generateNfsServer, namespace); err != nil {
		log.Error(err, "Validation failed")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle NfsServer lifecycle with callbacks
	return r.ReconcileNfsServerComplete(ctx, volumeObj, namespace, generateNfsServer, "sv", onReady, onNotReady)
}

// ContainsString checks if a slice contains a specific string
func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// RemoveString removes a specific string from a slice
func RemoveString(slice []string, s string) []string {
	result := []string{}
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}

// HandleFinalizerLogic handles the common finalizer pattern for volume objects.
// Returns true if reconciliation should continue, false if it should exit early.
func (r *VolumeControllerBase) HandleFinalizerLogic(ctx context.Context, obj client.Object, finalizerName string) (bool, error) {
	// Handle deletion
	if obj.GetDeletionTimestamp() != nil {
		return r.handleDeletion(ctx, obj, finalizerName)
	}

	// Add finalizer if not present
	return r.ensureFinalizer(ctx, obj, finalizerName)
}

// handleDeletion handles object deletion with cleanup and finalizer removal
func (r *VolumeControllerBase) handleDeletion(ctx context.Context, obj client.Object, finalizerName string) (bool, error) {
	log := logf.FromContext(ctx)

	if !ContainsString(obj.GetFinalizers(), finalizerName) {
		return false, nil
	}

	// Run cleanup logic
	if volumeObj, ok := obj.(VolumeObject); ok {
		if err := r.CleanupResources(ctx, volumeObj); err != nil {
			log.Error(err, "Failed to cleanup resources")
			return false, err
		}
	}

	// Remove the finalizer to allow deletion with retry logic
	if err := r.RemoveFinalizerWithRetry(ctx, obj, finalizerName); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return false, err
	}

	return false, nil
}

// ensureFinalizer adds finalizer if not present
func (r *VolumeControllerBase) ensureFinalizer(ctx context.Context, obj client.Object, finalizerName string) (bool, error) {
	log := logf.FromContext(ctx)

	if ContainsString(obj.GetFinalizers(), finalizerName) {
		return true, nil
	}

	finalizers := obj.GetFinalizers()
	finalizers = append(finalizers, finalizerName)
	obj.SetFinalizers(finalizers)
	if err := r.Update(ctx, obj); err != nil {
		log.Error(err, "Failed to add finalizer")
		return false, err
	}

	return true, nil
}

// EnsureAPIVersionAndKind ensures that APIVersion and Kind are set for owner references
func (r *VolumeControllerBase) EnsureAPIVersionAndKind(obj client.Object, apiVersion, kind string) {
	// For runtime objects, we need to set the fields directly
	// This is done by type assertion to the specific volume type
	// The caller will handle the type-specific logic

	// Set the TypeMeta fields through reflection if needed
	if runtimeObj, ok := obj.(interface{ SetAPIVersion(string) }); ok {
		if runtimeObj2, ok := obj.(interface{ GetAPIVersion() string }); ok {
			if runtimeObj2.GetAPIVersion() == "" {
				runtimeObj.SetAPIVersion(apiVersion)
			}
		}
	}

	if runtimeObj, ok := obj.(interface{ SetKind(string) }); ok {
		if runtimeObj2, ok := obj.(interface{ GetKind() string }); ok {
			if runtimeObj2.GetKind() == "" {
				runtimeObj.SetKind(kind)
			}
		}
	}
}

// WaitForPodsToTerminate waits for all pods related to a volume to fully terminate
// before proceeding with PVC deletion to avoid stuck pod termination
func (b *VolumeControllerBase) WaitForPodsToTerminate(ctx context.Context, volume VolumeObject, referenceValue string) {
	log := logf.FromContext(ctx)

	maxWaitTime := 60 * time.Second
	pollInterval := 2 * time.Second
	timeout := time.Now().Add(maxWaitTime)

	for time.Now().Before(timeout) {
		// Check for any remaining pods with the volume's label
		podList := &corev1.PodList{}
		if err := b.List(ctx, podList, client.InNamespace(volume.GetNamespace()), client.MatchingLabels{"app": referenceValue}); err != nil {
			log.Error(err, "Failed to list pods while waiting for termination", "referenceValue", referenceValue)
			return
		}

		if len(podList.Items) == 0 {
			log.Info("All pods have terminated, proceeding with PVC cleanup", "referenceValue", referenceValue)
			return
		}

		// Log remaining pods
		for _, pod := range podList.Items {
			log.Info("Waiting for pod to terminate", "pod", pod.Name, "phase", pod.Status.Phase, "deletionTimestamp", pod.DeletionTimestamp)
		}

		// Wait before checking again
		select {
		case <-time.After(pollInterval):
			continue
		case <-ctx.Done():
			log.Info("Context cancelled while waiting for pods to terminate")
			return
		}
	}

	log.Info("Timeout waiting for pods to terminate, proceeding with cleanup anyway", "referenceValue", referenceValue, "maxWaitTime", maxWaitTime)
}

// Generic callback creation functions

// CreateSyncReadyCallback creates a generic callback for when volume is ready that handles sync operations
func (r *VolumeControllerBase) CreateSyncReadyCallback() func(context.Context, VolumeObject) error {
	return func(ctx context.Context, volumeObj VolumeObject) error {
		// Start sync operations if sync controller is available and source is configured
		if r.SyncController != nil {
			// Check if this is a SharedVolume with source configuration
			if sv, ok := volumeObj.(*svv1alpha1.SharedVolume); ok && sv.Spec.Source != nil {
				if err := r.SyncController.StartSyncForSharedVolume(ctx, sv); err != nil {
					return err
				}
			}
			// Add support for other volume types here when needed
			// if csv, ok := volumeObj.(*svv1alpha1.ClusterSharedVolume); ok && csv.Spec.Source != nil {
			//     return r.SyncController.StartSyncForClusterSharedVolume(ctx, csv)
			// }
		}
		return nil
	}
}

// CreateSyncNotReadyCallback creates a generic callback for when volume is not ready that handles sync operations
func (r *VolumeControllerBase) CreateSyncNotReadyCallback() func(context.Context, VolumeObject) error {
	return func(ctx context.Context, volumeObj VolumeObject) error {
		// Stop sync operations if they were running
		if r.SyncController != nil {
			// Check if this is a SharedVolume
			if sv, ok := volumeObj.(*svv1alpha1.SharedVolume); ok {
				r.SyncController.StopSyncForSharedVolume(sv)
			}
			// Add support for other volume types here when needed
			// if csv, ok := volumeObj.(*svv1alpha1.ClusterSharedVolume); ok {
			//     r.SyncController.StopSyncForClusterSharedVolume(csv)
			// }
		}
		return nil
	}
}

// CreateNoOpReadyCallback creates a callback that does nothing when volume is ready
func (r *VolumeControllerBase) CreateNoOpReadyCallback() func(context.Context, VolumeObject) error {
	return func(ctx context.Context, volumeObj VolumeObject) error {
		return nil
	}
}

// CreateNoOpNotReadyCallback creates a callback that does nothing when volume is not ready
func (r *VolumeControllerBase) CreateNoOpNotReadyCallback() func(context.Context, VolumeObject) error {
	return func(ctx context.Context, volumeObj VolumeObject) error {
		return nil
	}
}

// Generic runnable and setup patterns

// OneTimeRecoveryRunnable is a generic runnable that executes a recovery operation once
type OneTimeRecoveryRunnable struct {
	name         string
	recoveryFunc func(context.Context) error
	recovered    bool
	needsLeader  bool
}

// NewOneTimeRecoveryRunnable creates a new one-time recovery runnable
func NewOneTimeRecoveryRunnable(name string, recoveryFunc func(context.Context) error, needsLeader bool) *OneTimeRecoveryRunnable {
	return &OneTimeRecoveryRunnable{
		name:         name,
		recoveryFunc: recoveryFunc,
		needsLeader:  needsLeader,
	}
}

// Start implements the Runnable interface
func (r *OneTimeRecoveryRunnable) Start(ctx context.Context) error {
	log := logf.FromContext(ctx).WithName(r.name)

	// Only run recovery once
	if r.recovered {
		log.Info("Recovery already completed, skipping")
		return nil
	}

	// Wait a moment for the cache to be fully synced
	select {
	case <-time.After(5 * time.Second):
		// Continue with recovery
	case <-ctx.Done():
		return ctx.Err()
	}

	log.Info("Starting recovery operation")

	if err := r.recoveryFunc(ctx); err != nil {
		log.Error(err, "Failed to execute recovery operation")
		// Don't return error - this shouldn't stop the controller
	}

	// Mark recovery as completed
	r.recovered = true

	// This runnable completes after recovery, so return nil to indicate completion
	return nil
}

// NeedLeaderElection returns whether this runnable needs leader election
func (r *OneTimeRecoveryRunnable) NeedLeaderElection() bool {
	return r.needsLeader
}

// ControllerSetupConfig holds configuration for generic controller setup
type ControllerSetupConfig struct {
	VolumeType          client.Object
	ControllerName      string
	ControllerNamespace string
	PreSetupHook        func(ctrl.Manager, *VolumeControllerBase) error
	Runnables           []manager.Runnable // Use controller-runtime manager.Runnable interface
}

// SetupGenericController provides a fully generic controller setup pattern
func (r *VolumeControllerBase) SetupGenericController(mgr ctrl.Manager, config ControllerSetupConfig, reconciler interface{}) error {
	// Set the controller namespace
	controllerNamespace := config.ControllerNamespace
	if controllerNamespace == "" {
		controllerNamespace = "shared-volume-controller"
	}

	// Initialize the base controller if not already done
	if r.Client == nil {
		*r = *NewVolumeControllerBase(mgr.GetClient(), mgr.GetScheme(), controllerNamespace, nil)
	}

	// Call pre-setup hook for controller-specific initialization
	if config.PreSetupHook != nil {
		if err := config.PreSetupHook(mgr, r); err != nil {
			// Log error but continue
			log := logf.FromContext(context.Background())
			log.Error(err, "Failed in pre-setup hook")
		}
	}

	// Add any additional runnables to the manager
	for _, runnable := range config.Runnables {
		if err := mgr.Add(runnable); err != nil {
			return fmt.Errorf("failed to add runnable to manager: %w", err)
		}
	}

	// Create and complete the controller builder with standard watches
	return ctrl.NewControllerManagedBy(mgr).
		For(config.VolumeType).
		Watches(&appsv1.ReplicaSet{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), config.VolumeType)).
		Watches(&corev1.PersistentVolumeClaim{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), config.VolumeType)).
		Watches(&corev1.Service{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), config.VolumeType)).
		Watches(&nfsv1alpha1.NfsServer{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), config.VolumeType)).
		Named(config.ControllerName).
		Complete(reconciler.(reconcile.Reconciler))
}

// CreateSyncControllerSetupHook creates a pre-setup hook for controllers that need sync operations
func (r *VolumeControllerBase) CreateSyncControllerSetupHook(createSyncController func(client.Client, *runtime.Scheme) *SyncController, recoveryFunc func(context.Context) error) func(ctrl.Manager, *VolumeControllerBase) error {
	return func(mgr ctrl.Manager, base *VolumeControllerBase) error {
		// Initialize sync controller
		syncController := createSyncController(mgr.GetClient(), mgr.GetScheme())

		// Update the base controller with sync controller
		base.SyncController = syncController

		// Add a recovery runnable if recovery function is provided
		if recoveryFunc != nil {
			recovery := NewOneTimeRecoveryRunnable("sync-recovery", recoveryFunc, true)
			if err := mgr.Add(recovery); err != nil {
				return err
			}
		}

		return nil
	}
}
