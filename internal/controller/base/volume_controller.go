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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/resources"
	"github.com/sharedvolume/shared-volume-controller/internal/controller/types"
	"github.com/sharedvolume/shared-volume-controller/internal/controller/utils"
)

// VolumeController manages the lifecycle of volume resources
type VolumeController struct {
	client.Client
	Scheme              *runtime.Scheme
	ControllerNamespace string

	// Composed resource managers
	NfsManager        *resources.NfsServerManager
	PVManager         *resources.PersistentVolumeManager
	PVCManager        *resources.PersistentVolumeClaimManager
	ReplicaSetManager *resources.ReplicaSetManager
	ServiceManager    *resources.ServiceManager
	ReadinessManager  *resources.ReadinessManager
	StatusManager     *StatusManager

	// Utility managers
	FinalizerManager *utils.FinalizerManager
	RetryManager     *utils.RetryManager
	RandomGen        *utils.RandomGenerator
	NameGen          *utils.NameGenerator

	// SyncController for sync operations (optional, set by controllers that need it)
	SyncController interface {
		RecoverSyncOperations(ctx context.Context) error
	}
}

// NewVolumeController creates a new VolumeController instance
func NewVolumeController(client client.Client, scheme *runtime.Scheme, controllerNamespace string) *VolumeController {
	return &VolumeController{
		Client:              client,
		Scheme:              scheme,
		ControllerNamespace: controllerNamespace,

		// Resource managers
		NfsManager:        resources.NewNfsServerManager(client),
		PVManager:         resources.NewPersistentVolumeManager(client),
		PVCManager:        resources.NewPersistentVolumeClaimManager(client),
		ReplicaSetManager: resources.NewReplicaSetManager(client),
		ServiceManager:    resources.NewServiceManager(client),
		ReadinessManager:  resources.NewReadinessManager(client),
		StatusManager:     NewStatusManager(client),

		// Utility managers
		FinalizerManager: utils.NewFinalizerManager(client),
		RetryManager:     utils.NewRetryManager(client),
		RandomGen:        utils.NewRandomGenerator(),
		NameGen:          utils.NewNameGenerator(),
	}
}

// ReconcileGeneric provides a fully generic reconciliation pattern
func (v *VolumeController) ReconcileGeneric(
	ctx context.Context,
	req ctrl.Request,
	config types.VolumeReconcileConfig,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Create a new volume object instance
	volumeObj := config.VolumeObjFactory()

	// Fetch the volume instance
	if err := v.Get(ctx, req.NamespacedName, volumeObj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle finalizer logic
	result, err := v.handleFinalizerLogic(ctx, volumeObj, config)
	if err != nil || result.Requeue || result.RequeueAfter > 0 {
		return result, err
	}

	// Only set to initializing state if not already in a valid phase
	currentPhase := volumeObj.GetPhase()
	if currentPhase == "" || currentPhase == types.VolumePhaseError {
		if err := v.StatusManager.SetInitializingStatus(ctx, volumeObj); err != nil {
			log.Error(err, "Failed to set initializing status")
		}
	}

	// Fill and validate spec
	if err := v.FillAndValidateSpec(volumeObj, config.Namespace); err != nil {
		log.Error(err, "Failed to fill and validate spec")
		if statusErr := v.StatusManager.SetErrorStatus(ctx, volumeObj, err.Error()); statusErr != nil {
			log.Error(statusErr, "Failed to set error status")
		}
		return ctrl.Result{}, err
	}

	// Handle NFS server lifecycle
	result, err = v.ReconcileNfsServerComplete(ctx, volumeObj, config.Namespace, config.CreateReadyCallback, config.CreateNotReadyCallback)
	if err != nil {
		log.Error(err, "Failed to reconcile NFS server")
		if statusErr := v.StatusManager.SetErrorStatus(ctx, volumeObj, err.Error()); statusErr != nil {
			log.Error(statusErr, "Failed to set error status")
		}
		return ctrl.Result{}, err
	}

	return result, nil
}

// handleFinalizerLogic manages finalizer addition and cleanup
func (v *VolumeController) handleFinalizerLogic(ctx context.Context, volumeObj types.VolumeObject, config types.VolumeReconcileConfig) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if volumeObj.GetDeletionTimestamp() != nil {
		// Object is being deleted
		log.Info("Volume is being deleted, starting cleanup")

		if err := v.CleanupResources(ctx, volumeObj, config.Namespace); err != nil {
			log.Error(err, "Failed to cleanup resources")
			return ctrl.Result{}, err
		}

		// Remove finalizer
		if err := v.FinalizerManager.RemoveFinalizerWithRetry(ctx, volumeObj, config.FinalizerName); err != nil {
			log.Error(err, "Failed to remove finalizer")
			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if err := v.FinalizerManager.AddFinalizerIfNotPresent(ctx, volumeObj, config.FinalizerName); err != nil {
		log.Error(err, "Failed to add finalizer")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// FillAndValidateSpec fills in default values and validates the volume spec
func (v *VolumeController) FillAndValidateSpec(volumeObj types.VolumeObject, namespace string) error {
	spec := volumeObj.GetVolumeSpec()

	// Generate reference value if not set
	if spec.ReferenceValue == "" {
		spec.ReferenceValue = v.NameGen.GenerateResourcePrefix(volumeObj.GetName(), 12)
	}

	// Set default storage capacity if not specified
	if spec.Storage.Capacity == "" {
		spec.Storage.Capacity = types.DefaultStorageCapacity
	}

	// Validate required fields
	if spec.Storage.Capacity == "" {
		return fmt.Errorf("storage capacity is required")
	}

	return nil
}

// ReconcileNfsServerComplete handles the complete NFS server lifecycle
func (v *VolumeController) ReconcileNfsServerComplete(
	ctx context.Context,
	volumeObj types.VolumeObject,
	namespace string,
	onReady func() func(context.Context, types.VolumeObject) error,
	onNotReady func() func(context.Context, types.VolumeObject) error,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	// Generate NFS server if not configured
	if spec.NfsServer == nil || spec.NfsServer.Name == "" {
		if err := v.StatusManager.SetCreatingNfsServerStatus(ctx, volumeObj); err != nil {
			log.Error(err, "Failed to set creating NFS server status")
		}

		result, err := v.NfsManager.GenerateAndCreateNfsServer(ctx, volumeObj, "sv", namespace)
		if err != nil {
			return result, err
		}
		if result.RequeueAfter > 0 {
			return result, nil
		}
	}

	// Check NFS server status
	nfsReady, err := v.NfsManager.IsReady(ctx, volumeObj, namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !nfsReady {
		if err := v.StatusManager.SetWaitingForNfsServerStatus(ctx, volumeObj); err != nil {
			log.Error(err, "Failed to set waiting for NFS server status")
		}
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	// NFS server is ready, reconcile all resources
	if err := v.StatusManager.SetCreatingResourcesStatus(ctx, volumeObj); err != nil {
		log.Error(err, "Failed to set creating resources status")
	}

	if err := v.ReconcileAllResources(ctx, volumeObj, namespace); err != nil {
		return ctrl.Result{}, err
	}

	// Check overall readiness
	return v.CheckResourceReadiness(ctx, volumeObj, namespace, onReady, onNotReady)
}

// ReconcileAllResources reconciles all volume resources
func (v *VolumeController) ReconcileAllResources(ctx context.Context, volumeObj types.VolumeObject, namespace string) error {
	// Reconcile in order: PV -> PVC -> ReplicaSet -> Service
	if err := v.PVManager.Reconcile(ctx, volumeObj, namespace); err != nil {
		return fmt.Errorf("failed to reconcile PV: %w", err)
	}

	if err := v.PVCManager.Reconcile(ctx, volumeObj, namespace); err != nil {
		return fmt.Errorf("failed to reconcile PVC: %w", err)
	}

	if err := v.ReplicaSetManager.Reconcile(ctx, volumeObj, namespace); err != nil {
		return fmt.Errorf("failed to reconcile ReplicaSet: %w", err)
	}

	if err := v.ServiceManager.Reconcile(ctx, volumeObj, namespace); err != nil {
		return fmt.Errorf("failed to reconcile Service: %w", err)
	}

	return nil
}

// CheckResourceReadiness checks if all resources are ready
func (v *VolumeController) CheckResourceReadiness(
	ctx context.Context,
	volumeObj types.VolumeObject,
	namespace string,
	onReady func() func(context.Context, types.VolumeObject) error,
	onNotReady func() func(context.Context, types.VolumeObject) error,
) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Check core resources readiness
	nfsReady, replicaSetReady, err := v.ReadinessManager.CheckCoreResources(ctx, volumeObj, namespace)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Create callback functions
	var readyCallback func(context.Context, types.VolumeObject) error
	var notReadyCallback func(context.Context, types.VolumeObject) error

	if onReady != nil {
		readyCallback = onReady()
	}
	if onNotReady != nil {
		notReadyCallback = onNotReady()
	}

	// Update status based on readiness
	if err := v.StatusManager.UpdateReadinessStatus(ctx, volumeObj, nfsReady, replicaSetReady, readyCallback, notReadyCallback); err != nil {
		return ctrl.Result{}, err
	}

	// If not ready, requeue
	if !nfsReady || !replicaSetReady {
		log.Info("Resources not ready, requeueing", "nfsReady", nfsReady, "replicaSetReady", replicaSetReady)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	log.Info("All resources are ready", "volume", volumeObj.GetName())
	return ctrl.Result{}, nil
}

// CleanupResources removes all resources associated with a volume
func (v *VolumeController) CleanupResources(ctx context.Context, volumeObj types.VolumeObject, namespace string) error {
	log := logf.FromContext(ctx)
	spec := volumeObj.GetVolumeSpec()

	if spec.ReferenceValue == "" {
		log.Info("No reference value, skipping resource cleanup")
		return nil
	}

	log.Info("Starting resource cleanup", "referenceValue", spec.ReferenceValue)

	// Delete in reverse order: Service -> ReplicaSet -> PVC -> PV
	// Note: NFS server cleanup should be handled by its own controller

	serviceName := v.ServiceManager.GenerateServiceName(spec.ReferenceValue)
	if err := v.ServiceManager.DeleteService(ctx, serviceName, namespace); err != nil {
		log.Error(err, "Failed to delete Service during cleanup")
		// Continue with other resources
	}

	replicaSetName := v.ReplicaSetManager.GenerateReplicaSetName(spec.ReferenceValue)
	if err := v.ReplicaSetManager.DeleteReplicaSet(ctx, replicaSetName, namespace); err != nil {
		log.Error(err, "Failed to delete ReplicaSet during cleanup")
		// Continue with other resources
	}

	pvcName := v.PVCManager.GeneratePVCName(spec.ReferenceValue)
	if err := v.PVCManager.DeletePersistentVolumeClaim(ctx, pvcName, namespace); err != nil {
		log.Error(err, "Failed to delete PVC during cleanup")
		// Continue with other resources
	}

	pvName := v.PVManager.GeneratePVName(spec.ReferenceValue)
	if err := v.PVManager.DeletePersistentVolume(ctx, pvName); err != nil {
		log.Error(err, "Failed to delete PV during cleanup")
		// Continue with other resources
	}

	log.Info("Resource cleanup completed", "referenceValue", spec.ReferenceValue)
	return nil
}

// EnsureControllerNamespace ensures that the controller namespace exists
func (v *VolumeController) EnsureControllerNamespace(ctx context.Context) error {
	log := logf.FromContext(ctx)

	ns := &corev1.Namespace{}
	err := v.Get(ctx, client.ObjectKey{Name: v.ControllerNamespace}, ns)

	if err == nil {
		return nil
	}

	if !apierrors.IsNotFound(err) {
		log.Error(err, "Failed to check if controller namespace exists")
		return err
	}

	// Create the namespace
	log.Info("Creating controller namespace", "namespace", v.ControllerNamespace)
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: v.ControllerNamespace,
		},
	}

	return v.Create(ctx, ns)
}

// CreateSyncReadyCallback creates a callback for when sync operations are ready
func (v *VolumeController) CreateSyncReadyCallback() func(context.Context, types.VolumeObject) error {
	return func(ctx context.Context, volumeObj types.VolumeObject) error {
		// Implementation for sync ready callback
		log := logf.FromContext(ctx)
		log.Info("Sync ready callback triggered", "volume", volumeObj.GetName())
		return nil
	}
}

// CreateSyncNotReadyCallback creates a callback for when sync operations are not ready
func (v *VolumeController) CreateSyncNotReadyCallback() func(context.Context, types.VolumeObject) error {
	return func(ctx context.Context, volumeObj types.VolumeObject) error {
		// Implementation for sync not ready callback
		log := logf.FromContext(ctx)
		log.Info("Sync not ready callback triggered", "volume", volumeObj.GetName())
		return nil
	}
}

// CreateSyncControllerSetupHook creates a setup hook for sync controller initialization
func (v *VolumeController) CreateSyncControllerSetupHook(
	syncControllerFactory interface{}, // Factory function for creating sync controller
	recoveryFunc func(context.Context) error, // Recovery function for sync operations
) func(ctrl.Manager, interface{}) error {
	return func(mgr ctrl.Manager, controller interface{}) error {
		log := logf.FromContext(context.Background())
		log.Info("Sync controller setup hook triggered")

		// Call recovery function if provided
		if recoveryFunc != nil {
			ctx := context.Background()
			if err := recoveryFunc(ctx); err != nil {
				log.Error(err, "Failed to recover sync operations")
				return err
			}
		}

		return nil
	}
}

// SetupGenericController sets up a generic controller with the provided configuration
func (v *VolumeController) SetupGenericController(mgr ctrl.Manager, config types.ControllerSetupConfig, controller interface{}) error {
	log := logf.FromContext(context.Background())
	log.Info("Setting up generic controller", "name", config.ControllerName)

	// Run pre-setup hook if provided
	if config.PreSetupHook != nil {
		if err := config.PreSetupHook(mgr, controller); err != nil {
			return fmt.Errorf("pre-setup hook failed: %w", err)
		}
	}

	// Add any additional runnables to the manager
	for _, runnable := range config.Runnables {
		if err := mgr.Add(runnable); err != nil {
			return fmt.Errorf("failed to add runnable to manager: %w", err)
		}
	}

	// Set up the controller with the manager
	builder := ctrl.NewControllerManagedBy(mgr).
		For(config.VolumeType).
		Named(config.ControllerName)

	// Complete the builder and return
	return builder.Complete(controller.(reconcile.Reconciler))
}
