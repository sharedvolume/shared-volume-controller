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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

// SharedVolumeReconciler reconciles a SharedVolume object
type SharedVolumeReconciler struct {
	*VolumeControllerBase
}

// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=sharedvolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=sharedvolumes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=sharedvolumes/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumeclaims/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=persistentvolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=persistentvolumes/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;list;watch;create
// +kubebuilder:rbac:groups=sharedvolume.io,resources=nfsservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups="",resources=pods/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SharedVolume object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *SharedVolumeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// NOTE: We use the SharedVolume's spec.NfsServer.Path value for the actual NFS server path
	// and for the share path in the PV. Make sure the path is valid and doesn't cause mounting issues.
	log := logf.FromContext(ctx)

	// Ensure the controller namespace exists
	if err := r.VolumeControllerBase.EnsureControllerNamespace(ctx); err != nil {
		log.Error(err, "Failed to ensure controller namespace exists")
		return ctrl.Result{}, err
	}

	// 1. Fetch the SharedVolume instance
	var sharedVolume svv1alpha1.SharedVolume
	if err := r.Get(ctx, req.NamespacedName, &sharedVolume); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure APIVersion and Kind are set for owner references
	if sharedVolume.APIVersion == "" {
		sharedVolume.APIVersion = "sv.sharedvolume.io/v1alpha1"
	}
	if sharedVolume.Kind == "" {
		sharedVolume.Kind = "SharedVolume"
	}

	// Define the finalizer name
	finalizerName := "shared-volume-controller.sharedvolume.io/finalizer"

	// Handle deletion - check if the object is being deleted
	if sharedVolume.DeletionTimestamp != nil {
		// The object is being deleted
		if ContainsString(sharedVolume.Finalizers, finalizerName) {
			// Run cleanup logic
			if err := r.VolumeControllerBase.CleanupResources(ctx, &sharedVolume); err != nil {
				log.Error(err, "Failed to cleanup resources")
				return ctrl.Result{}, err
			}

			// Remove the finalizer to allow deletion with retry logic
			if err := r.VolumeControllerBase.RemoveFinalizerWithRetry(ctx, &sharedVolume, finalizerName); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		// Exit reconciliation as the item is being deleted
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !ContainsString(sharedVolume.Finalizers, finalizerName) {
		sharedVolume.Finalizers = append(sharedVolume.Finalizers, finalizerName)
		if err := r.Update(ctx, &sharedVolume); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// Immutability enforcement has been removed and will be implemented separately

	// 3. Fill and validate spec
	generateNfsServer := sharedVolume.Spec.NfsServer == nil
	if err := r.VolumeControllerBase.FillAndValidateSpec(&sharedVolume, generateNfsServer); err != nil {
		log.Error(err, "Validation failed")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 4. Handle NfsServer lifecycle
	return r.reconcileNfsServer(ctx, &sharedVolume, generateNfsServer)
}

func (r *SharedVolumeReconciler) createAndOwnNfsServer(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	return r.VolumeControllerBase.CreateAndOwnNfsServer(ctx, sharedVolume)
}

// Not used anymore - we use checkNfsServerStatus instead which provides more information
// func (r *SharedVolumeReconciler) checkNfsServerReady(ctx context.Context, name, namespace string) (bool, string, error) {
// 	nfsServer := &nfsv1alpha1.NfsServer{}
// 	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, nfsServer)
// 	if err != nil {
// 		return false, "", err
// 	}
// 	return nfsServer.Status.Ready, nfsServer.Status.Phase, nil
// }

// Immutability checking has been removed and will be implemented separately

// reconcileNfsServer handles the NfsServer lifecycle management
func (r *SharedVolumeReconciler) reconcileNfsServer(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume, generateNfsServer bool) (ctrl.Result, error) {
	result, err := r.VolumeControllerBase.ReconcileNfsServer(ctx, sharedVolume, generateNfsServer)
	if err != nil {
		return result, err
	}

	// If NFS server is not ready yet, return the result from base
	if result.RequeueAfter > 0 {
		return result, nil
	}

	// NFS server is ready, continue with resource reconciliation
	if err := r.reconcileRequiredResources(ctx, sharedVolume); err != nil {
		return ctrl.Result{}, err
	}

	// Check if the resources are ready
	if err := r.VolumeControllerBase.CheckResourceReadiness(ctx, sharedVolume); err != nil {
		return ctrl.Result{}, err
	}

	// Only requeue if the SharedVolume is not ready yet
	if sharedVolume.Status.Phase != "Ready" {
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	return ctrl.Result{}, nil
}

// updateSharedVolumeStatus updates the status fields and returns true if any field was changed
func (r *SharedVolumeReconciler) updateSharedVolumeStatus(sharedVolume *svv1alpha1.SharedVolume, nfsReady bool, phase string, message string) bool {
	return r.VolumeControllerBase.UpdateSharedVolumeStatus(sharedVolume, nfsReady, phase, message)
}

// checkNfsServerStatus checks if the NfsServer exists and is ready
func (r *SharedVolumeReconciler) checkNfsServerStatus(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) (bool, string, string, error) {
	return r.VolumeControllerBase.CheckNfsServerStatus(ctx, sharedVolume)
}

// reconcileRequiredResources creates or updates the PV, PVC, ReplicaSet, and Service resources
func (r *SharedVolumeReconciler) reconcileRequiredResources(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	return r.VolumeControllerBase.ReconcileRequiredResources(ctx, sharedVolume)
}

// reconcilePersistentVolume creates or updates the PersistentVolume
func (r *SharedVolumeReconciler) reconcilePersistentVolume(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	return r.VolumeControllerBase.ReconcilePersistentVolume(ctx, sharedVolume)
}

// reconcilePersistentVolumeClaim creates or updates the PersistentVolumeClaim
func (r *SharedVolumeReconciler) reconcilePersistentVolumeClaim(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	return r.VolumeControllerBase.ReconcilePersistentVolumeClaim(ctx, sharedVolume)
}

// reconcileReplicaSet creates or updates the ReplicaSet
func (r *SharedVolumeReconciler) reconcileReplicaSet(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	return r.VolumeControllerBase.ReconcileReplicaSet(ctx, sharedVolume)
}

// reconcileService creates or updates the Service
func (r *SharedVolumeReconciler) reconcileService(ctx context.Context, sharedVolume *svv1alpha1.SharedVolume) error {
	return r.VolumeControllerBase.ReconcileService(ctx, sharedVolume)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SharedVolumeReconciler) SetupWithManager(mgr ctrl.Manager, controllerNamespace string) error {
	// Set the controller namespace
	if controllerNamespace == "" {
		// Default to "shared-volume-controller" if not specified
		controllerNamespace = "shared-volume-controller"
	}

	// Initialize sync controller
	syncController := NewSyncController(mgr.GetClient(), mgr.GetScheme())

	// Initialize the base controller
	r.VolumeControllerBase = NewVolumeControllerBase(mgr.GetClient(), mgr.GetScheme(), controllerNamespace, syncController)

	// Add a runnable to recover sync operations after the cache is synced
	if err := mgr.Add(&syncRecoveryRunnable{
		syncController: syncController,
	}); err != nil {
		return err
	}

	// Note: We don't create the namespace here because the cache isn't started yet.
	// The namespace will be created during the first reconciliation.

	return ctrl.NewControllerManagedBy(mgr).
		For(&svv1alpha1.SharedVolume{}).
		Watches(&appsv1.ReplicaSet{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &svv1alpha1.SharedVolume{})).
		Watches(&corev1.PersistentVolumeClaim{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &svv1alpha1.SharedVolume{})).
		Watches(&corev1.Service{},
			handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &svv1alpha1.SharedVolume{})).
		// TODO: Fix NfsServer watch - API group mismatch
		// Watches(&nfsv1alpha1.NfsServer{},
		//	handler.EnqueueRequestForOwner(mgr.GetScheme(), mgr.GetRESTMapper(), &svv1alpha1.SharedVolume{})).
		Named("sharedvolume").
		Complete(r)
}

// syncRecoveryRunnable is a runnable that recovers sync operations for existing SharedVolumes
// after the controller starts and the cache is synced
type syncRecoveryRunnable struct {
	syncController *SyncController
	recovered      bool // Flag to ensure recovery only runs once
}

// Start implements the Runnable interface
func (r *syncRecoveryRunnable) Start(ctx context.Context) error {
	log := logf.FromContext(ctx).WithName("sync-recovery")

	// Only run recovery once
	if r.recovered {
		log.Info("Sync recovery already completed, skipping")
		return nil
	}

	// Wait a moment for the cache to be fully synced
	select {
	case <-time.After(5 * time.Second):
		// Continue with recovery
	case <-ctx.Done():
		return ctx.Err()
	}

	log.Info("Starting sync recovery for existing SharedVolumes")

	if err := r.syncController.RecoverSyncOperations(ctx); err != nil {
		log.Error(err, "Failed to recover sync operations")
		// Don't return error - this shouldn't stop the controller
	}

	// Mark recovery as completed
	r.recovered = true

	// This runnable completes after recovery, so return nil to indicate completion
	return nil
}

// NeedLeaderElection returns true since sync recovery should only happen on the leader
func (r *syncRecoveryRunnable) NeedLeaderElection() bool {
	return true
}
