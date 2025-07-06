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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nfsv1alpha1 "github.com/sharedvolume/nfs-server-controller/api/v1alpha1"
	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

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

// FillAndValidateSpec fills defaults and validates the SharedVolume spec.
func (r *VolumeControllerBase) FillAndValidateSpec(sharedVolume *svv1alpha1.SharedVolume, generateNfsServer bool) error {
	if sharedVolume.Spec.ResourceNamespace == "" {
		sharedVolume.Spec.ResourceNamespace = sharedVolume.Namespace
	}
	if sharedVolume.Spec.MountPath == "" {
		return errors.New("mountPath is required in SharedVolume spec")
	}
	if sharedVolume.Spec.SyncInterval == "" {
		sharedVolume.Spec.SyncInterval = "60s"
	}
	if sharedVolume.Spec.SyncTimeout == "" {
		sharedVolume.Spec.SyncTimeout = "120s"
	}
	if sharedVolume.Spec.Storage == nil {
		return errors.New("storage is required in SharedVolume spec")
	}
	if sharedVolume.Spec.Storage.Capacity == "" {
		return errors.New("storage.capacity is required in SharedVolume spec")
	}
	if sharedVolume.Spec.Storage.AccessMode == "" {
		sharedVolume.Spec.Storage.AccessMode = "ReadOnly"
	} else if sharedVolume.Spec.Storage.AccessMode != "ReadWrite" && sharedVolume.Spec.Storage.AccessMode != "ReadOnly" {
		return errors.New("storage.accessMode must be either ReadWrite or ReadOnly in SharedVolume spec")
	}
	if generateNfsServer && sharedVolume.Spec.StorageClassName == "" {
		return errors.New("storageClassName is required if nfsServer is not defined")
	}
	// Ensure NfsServer has a Path if it's defined, defaulting to "/"
	// The actual NfsServer and PV will use this exact path for the share
	if !generateNfsServer && sharedVolume.Spec.NfsServer != nil {
		if sharedVolume.Spec.NfsServer.Path == "" {
			sharedVolume.Spec.NfsServer.Path = "/"
		}
	}
	return nil
}

// CreateAndOwnNfsServer creates an NfsServer resource and sets ownership
func (r *VolumeControllerBase) CreateAndOwnNfsServer(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	// Use the path from the spec for NfsServer path
	// We'll use spec.NfsServer.Path for the actual share path as requested

	nfsServer := &nfsv1alpha1.NfsServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      sharedVolume.Spec.NfsServer.Name,
			Namespace: sharedVolume.Namespace, // Use SharedVolume's namespace
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "shared-volume-controller",
				"shared-volume.io/reference":   sharedVolume.Spec.ReferenceValue,
				"shared-volume.io/owner":       fmt.Sprintf("%s.%s", sharedVolume.Name, sharedVolume.Namespace),
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: sharedVolume.APIVersion,
					Kind:       sharedVolume.Kind,
					Name:       sharedVolume.Name,
					UID:        sharedVolume.UID,
					Controller: pointer.Bool(false),
				},
			},
		},
		Spec: nfsv1alpha1.NfsServerSpec{
			Storage: nfsv1alpha1.StorageSpec{
				Capacity:         sharedVolume.Spec.Storage.Capacity,
				StorageClassName: sharedVolume.Spec.StorageClassName,
			},
			Image: sharedVolume.Spec.NfsServer.Image,
		},
	}
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
func (r *VolumeControllerBase) ReconcileNfsServer(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume, generateNfsServer bool) (ctrl.Result, error) {
	// If we need to generate an NfsServer
	if generateNfsServer {
		return r.GenerateAndCreateNfsServer(ctx, sharedVolume)
	}

	// Check if NfsServer exists and is ready
	if sharedVolume.Spec.NfsServer != nil && sharedVolume.Spec.NfsServer.Name != "" {
		return r.CheckAndUpdateNfsServerStatus(ctx, sharedVolume)
	}

	return ctrl.Result{}, nil
}

// GenerateAndCreateNfsServer generates and creates an NfsServer resource from the SharedVolume.
func (r *VolumeControllerBase) GenerateAndCreateNfsServer(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get the latest version of the object before updating
	var latestSharedVolume svv1alpha1.SharedVolume
	if err := r.Get(ctx, client.ObjectKeyFromObject(sharedVolume), &latestSharedVolume); err != nil {
		log.Error(err, "Failed to get latest SharedVolume before update")
		return ctrl.Result{}, err
	}

	// If someone else has already set the NfsServer, use that one instead
	if latestSharedVolume.Spec.NfsServer != nil {
		log.Info("NfsServer was already set", "name", latestSharedVolume.Spec.NfsServer.Name)
		*sharedVolume = latestSharedVolume
		return r.CheckAndUpdateNfsServerStatus(ctx, sharedVolume)
	}

	// Generate reference value for all related resources
	referenceValue := "sv-" + r.RandString(12)
	latestSharedVolume.Spec.ReferenceValue = referenceValue

	// Generate NfsServer spec
	nfsServerName := "nfs-" + referenceValue
	latestSharedVolume.Spec.NfsServer = &svv1alpha1.NfsServerSpec{
		Name:      nfsServerName,
		Namespace: sharedVolume.Namespace, // Use SharedVolume's namespace instead of ControllerNamespace
		URL:       nfsServerName + "." + sharedVolume.Namespace + ".svc.cluster.local",
		Path:      "/", // Default path, will be used as the actual share path
	}
	log.Info("Generating NfsServer", "name", nfsServerName)

	// Update with the new NfsServer spec
	if err := r.Update(ctx, &latestSharedVolume); err != nil {
		log.Error(err, "Failed to update SharedVolume with NfsServer spec")
		return ctrl.Result{}, err
	}

	// Update our reference with the latest version
	*sharedVolume = latestSharedVolume

	// Create the NfsServer resource
	if err := r.CreateAndOwnNfsServer(ctx, sharedVolume); err != nil {
		log.Error(err, "Failed to create NfsServer")
		return ctrl.Result{}, err
	}

	// Requeue to pick up the updated spec and avoid duplicate NfsServer creation
	return ctrl.Result{RequeueAfter: 2 * time.Second}, nil
}

// CheckAndUpdateNfsServerStatus checks NfsServer status and updates SharedVolume status accordingly
func (r *VolumeControllerBase) CheckAndUpdateNfsServerStatus(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	ready, phase, message, err := r.CheckNfsServerStatus(ctx, sharedVolume)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update the status fields
	statusChanged := r.UpdateSharedVolumeStatus(sharedVolume, ready, phase, message)

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

	// NfsServer is ready, return success for now
	// Resource reconciliation and readiness checking will be handled by the calling controller
	log.Info("NfsServer is ready, proceeding to resource reconciliation")
	return ctrl.Result{}, nil
}

// CheckNfsServerStatus checks if the NfsServer exists and is ready
func (r *VolumeControllerBase) CheckNfsServerStatus(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) (bool, string, string, error) {
	log := logf.FromContext(ctx)

	nfsServer := &nfsv1alpha1.NfsServer{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      sharedVolume.Spec.NfsServer.Name,
		Namespace: sharedVolume.Namespace, // Use SharedVolume's namespace
	}, nfsServer)

	if client.IgnoreNotFound(err) != nil {
		log.Error(err, "Error getting NfsServer")
		return false, "Error", "Failed to get NfsServer: " + err.Error(), err
	}

	if err != nil { // Not found
		message := "NfsServer not found, waiting for creation"
		log.Info(message,
			"name", sharedVolume.Spec.NfsServer.Name,
			"namespace", sharedVolume.Namespace)
		return false, "Pending", message, nil
	}

	if !nfsServer.Status.Ready {
		message := "NfsServer exists but is not ready yet"
		log.Info(message,
			"name", sharedVolume.Spec.NfsServer.Name,
			"status", nfsServer.Status.Phase)
		return false, "Pending", message, nil
	}

	// NFS server is ready - don't set the phase here, let checkResourceReadiness determine the final status
	return true, "", "NfsServer is ready", nil
}

// UpdateSharedVolumeStatus updates the status fields and returns true if any field was changed
func (r *VolumeControllerBase) UpdateSharedVolumeStatus(sharedVolume *svv1alpha1.SharedVolume, nfsReady bool, phase string, message string) bool {
	statusChanged := false

	// Update phase if changed and phase is not empty (empty phase means don't update)
	if phase != "" && sharedVolume.Status.Phase != phase {
		sharedVolume.Status.Phase = phase
		statusChanged = true
	}

	// Update message if changed
	if sharedVolume.Status.Message != message {
		sharedVolume.Status.Message = message
		statusChanged = true
	}

	// Set NFS server address if available and not already set
	if nfsReady && sharedVolume.Status.NfsServerAddress == "" && sharedVolume.Spec.NfsServer != nil && sharedVolume.Spec.NfsServer.URL != "" {
		sharedVolume.Status.NfsServerAddress = sharedVolume.Spec.NfsServer.URL
		statusChanged = true
	}

	// Note: Ready status will be updated by checkResourceReadiness based on overall readiness
	// Don't update Ready status here to avoid conflicts

	return statusChanged
}

// ReconcileRequiredResources creates or updates the PV, PVC, ReplicaSet, and Service resources
func (r *VolumeControllerBase) ReconcileRequiredResources(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	log := logf.FromContext(ctx)

	// If referenceValue is not set, generate one
	if sharedVolume.Spec.ReferenceValue == "" {
		sharedVolume.Spec.ReferenceValue = "sv-" + r.RandString(12)
		if err := r.Update(ctx, sharedVolume); err != nil {
			log.Error(err, "Failed to update SharedVolume with referenceValue")
			return err
		}
	}

	// Create PV
	if err := r.ReconcilePersistentVolume(ctx, sharedVolume); err != nil {
		return err
	}

	// Create PVC
	if err := r.ReconcilePersistentVolumeClaim(ctx, sharedVolume); err != nil {
		return err
	}

	// Create ReplicaSet
	if err := r.ReconcileReplicaSet(ctx, sharedVolume); err != nil {
		return err
	}

	// Create Service
	if err := r.ReconcileService(ctx, sharedVolume); err != nil {
		return err
	}

	return nil
}

// ReconcilePersistentVolume creates or updates the PersistentVolume
func (r *VolumeControllerBase) ReconcilePersistentVolume(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	log := logf.FromContext(ctx)

	// Define PV name
	pvName := fmt.Sprintf("pv-%s", sharedVolume.Spec.ReferenceValue)

	// Check if PV already exists
	existingPV := &corev1.PersistentVolume{}
	err := r.Get(ctx, client.ObjectKey{Name: pvName}, existingPV)
	if err == nil {
		// PV already exists, update status if needed
		if sharedVolume.Status.PersistentVolumeName != pvName {
			// Status update will be handled by the calling controller
			log.Info("PV already exists", "name", pvName)
		}
		return nil
	}

	// Get storage capacity as a resource quantity
	storageQuantity, err := resource.ParseQuantity(sharedVolume.Spec.Storage.Capacity)
	if err != nil {
		log.Error(err, "Failed to parse storage capacity", "capacity", sharedVolume.Spec.Storage.Capacity)
		return err
	}

	err = r.Get(ctx, client.ObjectKey{Name: pvName}, existingPV)
	if err == nil {
		// PV already exists, update status if needed
		if sharedVolume.Status.PersistentVolumeName != pvName {
			// Status update will be handled by the calling controller
			log.Info("PV already exists", "name", pvName)
		}
		return nil
	}

	// Define the PV
	// Use the path from the spec for the share
	// This needs to match the actual path used in the NFS server
	nfsSharePath := sharedVolume.Spec.NfsServer.Path

	pv := &corev1.PersistentVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name: pvName,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "shared-volume-controller",
				"shared-volume.io/reference":   sharedVolume.Spec.ReferenceValue,
				"shared-volume.io/owner":       fmt.Sprintf("%s.%s", sharedVolume.Name, sharedVolume.Namespace),
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
						sharedVolume.Spec.NfsServer.URL,
						nfsSharePath),
					VolumeAttributes: map[string]string{
						"server": sharedVolume.Spec.NfsServer.URL,
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
func (r *VolumeControllerBase) ReconcilePersistentVolumeClaim(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	log := logf.FromContext(ctx)

	// Define PVC name
	pvcName := sharedVolume.Spec.ReferenceValue
	pvName := fmt.Sprintf("pv-%s", sharedVolume.Spec.ReferenceValue)

	// Check if PVC already exists
	existingPVC := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      pvcName,
		Namespace: sharedVolume.Namespace, // Use SharedVolume's namespace
	}, existingPVC)

	if err == nil {
		// PVC already exists, update status if needed
		if sharedVolume.Status.PersistentVolumeClaimName != pvcName {
			// Status update will be handled by the calling controller
			log.Info("PVC already exists", "name", pvcName)
		}
		return nil
	}

	// Get storage capacity as a resource quantity
	storageQuantity, err := resource.ParseQuantity(sharedVolume.Spec.Storage.Capacity)
	if err != nil {
		log.Error(err, "Failed to parse storage capacity", "capacity", sharedVolume.Spec.Storage.Capacity)
		return err
	}

	// Define the PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: sharedVolume.Namespace, // Use SharedVolume's namespace
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "shared-volume-controller",
				"shared-volume.io/reference":   sharedVolume.Spec.ReferenceValue,
				"shared-volume.io/owner":       fmt.Sprintf("%s.%s", sharedVolume.Name, sharedVolume.Namespace),
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: sharedVolume.APIVersion,
					Kind:       sharedVolume.Kind,
					Name:       sharedVolume.Name,
					UID:        sharedVolume.UID,
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
func (r *VolumeControllerBase) ReconcileReplicaSet(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	log := logf.FromContext(ctx)

	// Define ReplicaSet name
	replicaSetName := sharedVolume.Spec.ReferenceValue
	pvcName := sharedVolume.Spec.ReferenceValue

	// Check if ReplicaSet already exists
	existingRS := &appsv1.ReplicaSet{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      replicaSetName,
		Namespace: sharedVolume.Namespace, // Use SharedVolume's namespace
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
			Namespace: sharedVolume.Namespace, // Use SharedVolume's namespace
			Labels: map[string]string{
				"app":                          replicaSetName,
				"app.kubernetes.io/managed-by": "shared-volume-controller",
				"shared-volume.io/reference":   sharedVolume.Spec.ReferenceValue,
				"shared-volume.io/owner":       fmt.Sprintf("%s.%s", sharedVolume.Name, sharedVolume.Namespace),
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: sharedVolume.APIVersion,
					Kind:       sharedVolume.Kind,
					Name:       sharedVolume.Name,
					UID:        sharedVolume.UID,
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
								"-c",
								fmt.Sprintf("mkdir -p /nfs/%s-%s && echo 'sv-sample-file' > /nfs/%s-%s/.sv && echo 'Created folder /nfs/%s-%s with .sv file'",
									sharedVolume.Name, sharedVolume.Namespace,
									sharedVolume.Name, sharedVolume.Namespace,
									sharedVolume.Name, sharedVolume.Namespace),
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
							Name:  sharedVolume.Spec.ReferenceValue + "-syncer",
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

	log.Info("Created ReplicaSet", "name", replicaSetName, "referenceID", sharedVolume.Spec.ReferenceValue)
	return nil
}

// ReconcileService creates or updates the Service
func (r *VolumeControllerBase) ReconcileService(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	log := logf.FromContext(ctx)

	// Define Service name
	serviceName := fmt.Sprintf("%s", sharedVolume.Spec.ReferenceValue)
	replicaSetName := sharedVolume.Spec.ReferenceValue

	// Check if Service already exists
	existingSvc := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      serviceName,
		Namespace: sharedVolume.Namespace, // Use SharedVolume's namespace
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
			Namespace: sharedVolume.Namespace, // Use SharedVolume's namespace
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "shared-volume-controller",
				"shared-volume.io/reference":   sharedVolume.Spec.ReferenceValue,
				"shared-volume.io/owner":       fmt.Sprintf("%s.%s", sharedVolume.Name, sharedVolume.Namespace),
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: sharedVolume.APIVersion,
					Kind:       sharedVolume.Kind,
					Name:       sharedVolume.Name,
					UID:        sharedVolume.UID,
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

// CleanupResources removes all resources associated with a SharedVolume
func (b *VolumeControllerBase) CleanupResources(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	log := logf.FromContext(ctx)

	// Stop sync operations first
	if b.SyncController != nil {
		b.SyncController.StopSyncForSharedVolume(sharedVolume)
	}

	if sharedVolume.Spec.ReferenceValue == "" {
		return nil
	}

	referenceValue := sharedVolume.Spec.ReferenceValue

	log.Info("Starting comprehensive cleanup for SharedVolume", "name", sharedVolume.Name, "referenceValue", referenceValue)

	// 1. Find and force delete ALL pods that use this SharedVolume (not just ReplicaSet pods)
	b.CleanupAllPodsUsingSharedVolume(ctx, sharedVolume)

	// 2. Delete ReplicaSet and any remaining pods
	b.CleanupReplicaSet(ctx, sharedVolume, referenceValue)

	// 3. Delete Service
	b.CleanupService(ctx, sharedVolume, referenceValue)

	// 4. Delete all PVCs related to this SharedVolume (both main and namespace-specific ones)
	b.CleanupAllPVCs(ctx, sharedVolume, referenceValue)

	// 5. Delete all PVs related to this SharedVolume (both main and namespace-specific ones)
	b.CleanupAllPVs(ctx, sharedVolume, referenceValue)

	// 6. Delete NFS Server if it was generated
	b.CleanupNFSServer(ctx, sharedVolume)

	log.Info("Completed comprehensive cleanup for SharedVolume", "name", sharedVolume.Name)
	return nil
}

// CleanupAllPodsUsingSharedVolume finds and force deletes all pods that use this SharedVolume
func (b *VolumeControllerBase) CleanupAllPodsUsingSharedVolume(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) {
	log := logf.FromContext(ctx)

	// Search across all namespaces for pods using this SharedVolume
	podList := &corev1.PodList{}
	if err := b.List(ctx, podList); err != nil {
		log.Error(err, "Failed to list pods for SharedVolume cleanup")
		return
	}

	sharedVolumeName := sharedVolume.Name
	sharedVolumeNamespace := sharedVolume.Namespace

	for _, pod := range podList.Items {
		// Check if pod has SharedVolume annotations
		if pod.Annotations != nil {
			for key, value := range pod.Annotations {
				if strings.HasPrefix(key, "sharedvolume.sv/") && value == "true" {
					// Extract the SharedVolume reference from the annotation key
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

					// Check if this pod references our SharedVolume
					if name == sharedVolumeName && namespace == sharedVolumeNamespace {
						log.Info("Found pod using SharedVolume, force deleting",
							"pod", pod.Name,
							"podNamespace", pod.Namespace,
							"sharedVolume", sharedVolumeName)

						if err := b.ForceDeletePod(ctx, &pod); err != nil {
							log.Error(err, "Failed to force delete pod using SharedVolume",
								"pod", pod.Name, "namespace", pod.Namespace)
						}
					}
				}
			}
		}
	}
}

// CleanupReplicaSet deletes the ReplicaSet and any remaining pods
func (b *VolumeControllerBase) CleanupReplicaSet(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume, referenceValue string) {
	log := logf.FromContext(ctx)

	rs := &appsv1.ReplicaSet{}
	if err := b.Get(ctx, client.ObjectKey{
		Name:      referenceValue,
		Namespace: sharedVolume.Namespace,
	}, rs); err == nil {
		// First, explicitly delete all pods owned by this ReplicaSet
		podList := &corev1.PodList{}
		if err := b.List(ctx, podList, client.InNamespace(sharedVolume.Namespace), client.MatchingLabels{"app": referenceValue}); err == nil {
			for _, pod := range podList.Items {
				log.Info("Force deleting ReplicaSet pod", "name", pod.Name, "namespace", pod.Namespace)
				if err := b.ForceDeletePod(ctx, &pod); err != nil {
					log.Error(err, "Failed to force delete ReplicaSet pod", "name", pod.Name)
				}
			}
		}

		// Then delete the ReplicaSet
		log.Info("Deleting ReplicaSet", "name", referenceValue, "namespace", sharedVolume.Namespace)
		if err := b.Delete(ctx, rs); err != nil {
			log.Error(err, "Failed to delete ReplicaSet", "name", referenceValue)
		}
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get ReplicaSet", "name", referenceValue)
	}
}

// CleanupService deletes the service
func (b *VolumeControllerBase) CleanupService(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume, referenceValue string) {
	log := logf.FromContext(ctx)

	svcName := fmt.Sprintf("%s", referenceValue)
	svc := &corev1.Service{}
	if err := b.Get(ctx, client.ObjectKey{
		Name:      svcName,
		Namespace: sharedVolume.Namespace,
	}, svc); err == nil {
		log.Info("Deleting Service", "name", svcName, "namespace", sharedVolume.Namespace)
		if err := b.Delete(ctx, svc); err != nil {
			log.Error(err, "Failed to delete Service", "name", svcName)
		}
	} else if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to get Service", "name", svcName)
	}
}

// CleanupAllPVCs deletes all PVCs related to this SharedVolume
func (b *VolumeControllerBase) CleanupAllPVCs(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume, referenceValue string) {
	log := logf.FromContext(ctx)

	// 1. Delete the main PVC
	pvc := &corev1.PersistentVolumeClaim{}
	if err := b.Get(ctx, client.ObjectKey{
		Name:      referenceValue,
		Namespace: sharedVolume.Namespace,
	}, pvc); err == nil {
		log.Info("Force deleting main PVC", "name", referenceValue, "namespace", sharedVolume.Namespace)
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

// CleanupAllPVs deletes all PVs related to this SharedVolume
func (b *VolumeControllerBase) CleanupAllPVs(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume, referenceValue string) {
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
func (b *VolumeControllerBase) CleanupNFSServer(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) {
	log := logf.FromContext(ctx)

	if sharedVolume.Spec.NfsServer != nil && sharedVolume.Spec.NfsServer.Name != "" {
		nfsServer := &nfsv1alpha1.NfsServer{}
		if err := b.Get(ctx, client.ObjectKey{
			Name:      sharedVolume.Spec.NfsServer.Name,
			Namespace: sharedVolume.Namespace,
		}, nfsServer); err == nil {
			log.Info("Deleting NFS Server", "name", sharedVolume.Spec.NfsServer.Name, "namespace", sharedVolume.Namespace)
			if err := b.Delete(ctx, nfsServer); err != nil {
				log.Error(err, "Failed to delete NfsServer", "name", sharedVolume.Spec.NfsServer.Name)
			}
		} else if !apierrors.IsNotFound(err) {
			log.Error(err, "Failed to get NFS Server", "name", sharedVolume.Spec.NfsServer.Name)
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

// RemoveFinalizerWithRetry removes a finalizer from the SharedVolume with retry and conflict handling
func (b *VolumeControllerBase) RemoveFinalizerWithRetry(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume, finalizerName string) error {
	log := logf.FromContext(ctx)

	// Maximum number of retries
	maxRetries := 3
	retryCount := 0

	for {
		// Get the latest version of the object before updating to prevent conflicts
		var latestSharedVolume svv1alpha1.SharedVolume
		if err := b.Get(ctx, client.ObjectKeyFromObject(sharedVolume), &latestSharedVolume); err != nil {
			if apierrors.IsNotFound(err) {
				// Object already deleted, nothing to do
				log.Info("SharedVolume already deleted, no need to remove finalizer")
				return nil
			}
			log.Error(err, "Failed to get latest SharedVolume before finalizer removal")
			return err
		}

		// Check if the finalizer is still present
		if !ContainsString(latestSharedVolume.Finalizers, finalizerName) {
			log.Info("Finalizer already removed")
			*sharedVolume = latestSharedVolume
			return nil
		}

		// Remove the finalizer from the latest version
		latestSharedVolume.Finalizers = RemoveString(latestSharedVolume.Finalizers, finalizerName)

		// Update the object
		if err := b.Update(ctx, &latestSharedVolume); err != nil {
			if apierrors.IsConflict(err) && retryCount < maxRetries {
				retryCount++
				log.Info("Conflict detected while removing finalizer, retrying", "retryCount", retryCount)
				time.Sleep(time.Millisecond * 100 * time.Duration(retryCount)) // Backoff with each retry
				continue
			}
			if apierrors.IsNotFound(err) {
				// Object was deleted during our retry, which is fine
				log.Info("SharedVolume was deleted during finalizer removal")
				return nil
			}
			log.Error(err, "Failed to remove finalizer after retries")
			return err
		}

		// Success - update our reference with the latest version
		*sharedVolume = latestSharedVolume
		log.Info("Successfully removed finalizer", "finalizer", finalizerName)
		return nil
	}
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

// CheckResourceReadiness checks the readiness of all resources associated with a SharedVolume
func (b *VolumeControllerBase) CheckResourceReadiness(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	log := logf.FromContext(ctx)

	// If reference value is not set, return
	if sharedVolume.Spec.ReferenceValue == "" {
		return nil
	}

	// First check if NFS server is ready
	nfsReady, _, _, err := b.CheckNfsServerStatus(ctx, sharedVolume)
	if err != nil {
		return err
	}

	if !nfsReady {
		log.Info("NFS server not ready, marking SharedVolume as not ready")
		if b.ShouldUpdateReadinessStatus(sharedVolume, false, false) {
			if err := b.UpdateReadinessStatus(ctx, sharedVolume, false, false); err != nil {
				return err
			}
		}
		return nil
	}

	// Check if PVC is bound
	pvcReady, err := b.IsPVCReady(ctx, sharedVolume)
	if err != nil {
		return err
	}

	if !pvcReady {
		log.Info("PVC not ready yet", "name", sharedVolume.Spec.ReferenceValue)
		// Don't check ReplicaSet if PVC is not ready, just update status to not ready
		if b.ShouldUpdateReadinessStatus(sharedVolume, nfsReady, false) {
			if err := b.UpdateReadinessStatus(ctx, sharedVolume, nfsReady, false); err != nil {
				return err
			}
		}
		return nil
	}

	// Check if the ReplicaSet is ready
	replicaSetReady, err := b.IsReplicaSetReady(ctx, sharedVolume)
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
	if b.ShouldUpdateReadinessStatus(sharedVolume, nfsReady, replicaSetReady) {
		log.Info("Resource readiness changed, updating status",
			"currentPhase", sharedVolume.Status.Phase,
			"expectedPhase", b.DeterminePhase(nfsReady, replicaSetReady),
			"nfsReady", nfsReady,
			"replicaSetReady", replicaSetReady)
		// Update the status
		if err := b.UpdateReadinessStatus(ctx, sharedVolume, nfsReady, replicaSetReady); err != nil {
			return err
		}
	} else {
		log.V(1).Info("Resource readiness unchanged, skipping status update",
			"phase", sharedVolume.Status.Phase,
			"nfsReady", nfsReady,
			"replicaSetReady", replicaSetReady)
	}

	return nil
}

// IsReplicaSetReady checks if the ReplicaSet associated with the SharedVolume is ready
func (b *VolumeControllerBase) IsReplicaSetReady(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) (bool, error) {
	log := logf.FromContext(ctx)

	// Get the ReplicaSet using the reference value
	replicaSetName := sharedVolume.Spec.ReferenceValue
	rs := &appsv1.ReplicaSet{}
	err := b.Get(ctx, client.ObjectKey{
		Name:      replicaSetName,
		Namespace: sharedVolume.Namespace, // Use SharedVolume's namespace
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

// IsPVCReady checks if the PVC associated with the SharedVolume is bound
func (b *VolumeControllerBase) IsPVCReady(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) (bool, error) {
	log := logf.FromContext(ctx)

	pvcName := sharedVolume.Spec.ReferenceValue
	pvc := &corev1.PersistentVolumeClaim{}
	err := b.Get(ctx, client.ObjectKey{
		Name:      pvcName,
		Namespace: sharedVolume.Namespace, // Use SharedVolume's namespace
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

// ShouldUpdateReadinessStatus determines if a status update is needed based on the current status
func (b *VolumeControllerBase) ShouldUpdateReadinessStatus(sharedVolume *svv1alpha1.SharedVolume, nfsReady, replicaSetReady bool) bool {
	currentPhase := sharedVolume.Status.Phase
	expectedPhase := b.DeterminePhase(nfsReady, replicaSetReady)
	return currentPhase != expectedPhase
}

// DeterminePhase determines the correct phase based on NFS and ReplicaSet readiness
func (b *VolumeControllerBase) DeterminePhase(nfsReady, replicaSetReady bool) string {
	if nfsReady && replicaSetReady {
		return "Ready"
	} else if nfsReady && !replicaSetReady {
		return "Preparing"
	} else {
		return "Pending"
	}
}

// UpdateReadinessStatus updates the SharedVolume status based on NFS and ReplicaSet readiness
func (b *VolumeControllerBase) UpdateReadinessStatus(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume, nfsReady, replicaSetReady bool) error {
	log := logf.FromContext(ctx)

	phase := b.DeterminePhase(nfsReady, replicaSetReady)
	overallReady := phase == "Ready"

	err := b.UpdateStatusWithRetry(ctx, sharedVolume, func(sv *svv1alpha1.SharedVolume) {
		sv.Status.Phase = phase
		switch phase {
		case "Ready":
			sv.Status.Message = "SharedVolume is ready for use"
		case "Preparing":
			sv.Status.Message = "NFS server is ready, waiting for ReplicaSet to be ready"
		case "Pending":
			if !nfsReady {
				sv.Status.Message = "Waiting for NFS server to be ready"
			} else {
				sv.Status.Message = "Waiting for resources to be ready"
			}
		}
	})

	if err != nil {
		log.Error(err, "Failed to update SharedVolume status")
		return err
	}

	if overallReady {
		log.Info("SharedVolume resources are ready",
			"name", sharedVolume.Name,
			"referenceID", sharedVolume.Spec.ReferenceValue,
			"phase", phase)

		// Start sync operations if source is configured and sync controller is available
		if b.SyncController != nil && sharedVolume.Spec.Source != nil {
			if err := b.SyncController.StartSyncForSharedVolume(ctx, sharedVolume); err != nil {
				log.Error(err, "Failed to start sync operations")
				// Don't return error, sync is not critical for resource readiness
			}
		}
	} else {
		log.Info("SharedVolume is not fully ready",
			"name", sharedVolume.Name,
			"referenceID", sharedVolume.Spec.ReferenceValue,
			"phase", phase,
			"nfsReady", nfsReady,
			"replicaSetReady", replicaSetReady)

		// Stop sync operations if they were running
		if b.SyncController != nil {
			b.SyncController.StopSyncForSharedVolume(sharedVolume)
		}
	}

	return nil
}

// UpdateStatusWithRetry updates the status of the SharedVolume with retry and conflict handling
func (b *VolumeControllerBase) UpdateStatusWithRetry(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume,
	updateFn func(sv *svv1alpha1.SharedVolume)) error {
	log := logf.FromContext(ctx)

	// Maximum number of retries
	maxRetries := 3
	retryCount := 0

	for {
		// Get the latest version of the object before updating to prevent conflicts
		var latestSharedVolume svv1alpha1.SharedVolume
		if err := b.Get(ctx, client.ObjectKeyFromObject(sharedVolume), &latestSharedVolume); err != nil {
			log.Error(err, "Failed to get latest SharedVolume before status update")
			return err
		}

		// Apply the status update function to the latest object
		updateFn(&latestSharedVolume)

		// Update the status on the latest version of the object
		if err := b.Status().Update(ctx, &latestSharedVolume); err != nil {
			if apierrors.IsConflict(err) && retryCount < maxRetries {
				retryCount++
				log.Info("Conflict detected while updating status, retrying", "retryCount", retryCount)
				time.Sleep(time.Millisecond * 100 * time.Duration(retryCount)) // Backoff with each retry
				continue
			}
			log.Error(err, "Failed to update SharedVolume status after retries")
			return err
		}

		// Success - update our reference with the latest version
		*sharedVolume = latestSharedVolume
		return nil
	}
}
