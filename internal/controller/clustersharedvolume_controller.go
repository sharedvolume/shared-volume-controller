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

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

const (
	// OperationNamespace is the namespace used for ClusterSharedVolume operations
	OperationNamespace = "shared-volume-controller-operation"
)

// ClusterSharedVolumeReconciler reconciles a ClusterSharedVolume object
type ClusterSharedVolumeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=clustersharedvolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=clustersharedvolumes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=clustersharedvolumes/finalizers,verbs=update
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
func (r *ClusterSharedVolumeReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Ensure the operation namespace exists
	if err := r.ensureOperationNamespace(ctx); err != nil {
		log.Error(err, "Failed to ensure operation namespace exists")
		return ctrl.Result{}, err
	}

	// 1. Fetch the ClusterSharedVolume instance
	var clusterSharedVolume svv1alpha1.ClusterSharedVolume
	if err := r.Get(ctx, req.NamespacedName, &clusterSharedVolume); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Ensure APIVersion and Kind are set for owner references
	if clusterSharedVolume.APIVersion == "" {
		clusterSharedVolume.APIVersion = "sv.sharedvolume.io/v1alpha1"
	}
	if clusterSharedVolume.Kind == "" {
		clusterSharedVolume.Kind = "ClusterSharedVolume"
	}

	// Define the finalizer name
	finalizerName := "shared-volume-controller.sharedvolume.io/clustersharedvolume-finalizer"

	// Handle deletion
	if clusterSharedVolume.DeletionTimestamp != nil {
		if ContainsString(clusterSharedVolume.Finalizers, finalizerName) {
			// Since we use owner references, Kubernetes will automatically delete the SharedVolume
			// We just need to remove our finalizer to allow deletion
			log.Info("ClusterSharedVolume is being deleted, removing finalizer", "name", clusterSharedVolume.Name)

			// Remove the finalizer
			clusterSharedVolume.Finalizers = RemoveString(clusterSharedVolume.Finalizers, finalizerName)
			if err := r.Update(ctx, &clusterSharedVolume); err != nil {
				log.Error(err, "Failed to remove finalizer")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !ContainsString(clusterSharedVolume.Finalizers, finalizerName) {
		clusterSharedVolume.Finalizers = append(clusterSharedVolume.Finalizers, finalizerName)
		if err := r.Update(ctx, &clusterSharedVolume); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
	}

	// 2. Create or update the corresponding SharedVolume
	if err := r.ensureSharedVolume(ctx, &clusterSharedVolume); err != nil {
		log.Error(err, "Failed to ensure SharedVolume exists")
		return ctrl.Result{}, err
	}

	// 3. Update ClusterSharedVolume status based on SharedVolume status
	if err := r.updateStatusFromSharedVolume(ctx, &clusterSharedVolume); err != nil {
		log.Error(err, "Failed to update ClusterSharedVolume status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// ensureSharedVolume creates or updates a SharedVolume in the operation namespace
func (r *ClusterSharedVolumeReconciler) ensureSharedVolume(ctx context.Context, csv *svv1alpha1.ClusterSharedVolume) error {
	log := logf.FromContext(ctx)

	sharedVolume := &svv1alpha1.SharedVolume{
		ObjectMeta: metav1.ObjectMeta{
			Name:      csv.Name,
			Namespace: OperationNamespace,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         csv.APIVersion,
					Kind:               csv.Kind,
					Name:               csv.Name,
					UID:                csv.UID,
					Controller:         &[]bool{true}[0],
					BlockOwnerDeletion: &[]bool{true}[0],
				},
			},
		},
		Spec: svv1alpha1.SharedVolumeSpec{
			VolumeSpecBase: csv.Spec.VolumeSpecBase,
		},
	}

	// Check if SharedVolume exists
	existing := &svv1alpha1.SharedVolume{}
	err := r.Get(ctx, client.ObjectKeyFromObject(sharedVolume), existing)

	if err == nil {
		// Update existing SharedVolume
		existing.Spec = sharedVolume.Spec
		// Also update owner references if they've changed
		existing.OwnerReferences = sharedVolume.OwnerReferences
		return r.Update(ctx, existing)
	}

	if apierrors.IsNotFound(err) {
		// Create new SharedVolume
		log.Info("Creating SharedVolume with owner reference", "name", sharedVolume.Name, "namespace", sharedVolume.Namespace, "owner", csv.Name)
		return r.Create(ctx, sharedVolume)
	}

	return err
}

// updateStatusFromSharedVolume updates CSV status based on SharedVolume status
func (r *ClusterSharedVolumeReconciler) updateStatusFromSharedVolume(ctx context.Context, csv *svv1alpha1.ClusterSharedVolume) error {
	// Get the SharedVolume status
	sharedVolume := &svv1alpha1.SharedVolume{}
	err := r.Get(ctx, client.ObjectKey{Name: csv.Name, Namespace: OperationNamespace}, sharedVolume)
	if apierrors.IsNotFound(err) {
		// SharedVolume not found, set CSV status to pending
		csv.Status.Phase = "Pending"
		csv.Status.Message = "SharedVolume not created yet"
		return r.Status().Update(ctx, csv)
	}
	if err != nil {
		return err
	}

	// Copy status from SharedVolume to ClusterSharedVolume
	csv.Status.Phase = sharedVolume.Status.Phase
	csv.Status.Message = sharedVolume.Status.Message
	csv.Status.NfsServerAddress = sharedVolume.Status.NfsServerAddress
	csv.Status.PersistentVolumeClaimName = sharedVolume.Status.PersistentVolumeClaimName
	csv.Status.PersistentVolumeName = sharedVolume.Status.PersistentVolumeName
	csv.Status.ServiceName = sharedVolume.Status.ServiceName

	return r.Status().Update(ctx, csv)
}

// ensureOperationNamespace ensures that the operation namespace exists
func (r *ClusterSharedVolumeReconciler) ensureOperationNamespace(ctx context.Context) error {
	log := logf.FromContext(ctx)

	ns := &corev1.Namespace{}
	err := r.Get(ctx, client.ObjectKey{Name: OperationNamespace}, ns)

	if err == nil {
		return nil // Namespace exists
	}

	if !apierrors.IsNotFound(err) {
		return err // Some other error
	}

	// Create the namespace
	log.Info("Creating operation namespace", "namespace", OperationNamespace)
	ns = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: OperationNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "shared-volume-controller",
				"app.kubernetes.io/component":  "operation-namespace",
				"app.kubernetes.io/managed-by": "shared-volume-controller",
			},
		},
	}

	return r.Create(ctx, ns)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterSharedVolumeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&svv1alpha1.ClusterSharedVolume{}).
		Complete(r)
}
