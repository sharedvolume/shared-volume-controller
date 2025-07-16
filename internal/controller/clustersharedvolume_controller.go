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

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

const (
	// ClusterSharedVolumeOperationNamespace is the namespace used for ClusterSharedVolume operations
	ClusterSharedVolumeOperationNamespace = "shared-volume-controller"
)

// ClusterSharedVolumeReconciler reconciles a ClusterSharedVolume object
type ClusterSharedVolumeReconciler struct {
	*VolumeControllerBase
}

// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=clustersharedvolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=clustersharedvolumes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=clustersharedvolumes/finalizers,verbs=update
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
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *ClusterSharedVolumeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Use the fully generic reconciliation pattern with static namespace
	return r.VolumeControllerBase.ReconcileGeneric(ctx, req, VolumeReconcileConfig{
		VolumeObjFactory: func() VolumeObject {
			return &svv1alpha1.ClusterSharedVolume{}
		},
		APIVersion:    "sv.sharedvolume.io/v1alpha1",
		Kind:          "ClusterSharedVolume",
		FinalizerName: "shared-volume-controller.sharedvolume.io/clustersharedvolume-finalizer",
		Namespace:     ClusterSharedVolumeOperationNamespace, // Static namespace for cluster-scoped resources
		CreateReadyCallback: func() func(context.Context, VolumeObject) error {
			return r.VolumeControllerBase.CreateSyncReadyCallback()
		},
		CreateNotReadyCallback: func() func(context.Context, VolumeObject) error {
			return r.VolumeControllerBase.CreateSyncNotReadyCallback()
		},
	})
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterSharedVolumeReconciler) SetupWithManager(mgr ctrl.Manager, controllerNamespace string) error {
	// Create sync controller setup hook using the generic pattern
	preSetupHook := r.VolumeControllerBase.CreateSyncControllerSetupHook(
		NewSyncController, // Sync controller factory function
		func(ctx context.Context) error {
			// Recovery function for sync operations
			if r.VolumeControllerBase.SyncController != nil {
				return r.VolumeControllerBase.SyncController.RecoverSyncOperations(ctx)
			}
			return nil
		},
	)

	// Use the fully generic controller setup pattern
	return r.VolumeControllerBase.SetupGenericController(mgr, ControllerSetupConfig{
		VolumeType:          &svv1alpha1.ClusterSharedVolume{},
		ControllerName:      "clustersharedvolume",
		ControllerNamespace: controllerNamespace,
		PreSetupHook:        preSetupHook,
		Runnables:           []manager.Runnable{}, // No additional runnables needed
	}, r)
}
