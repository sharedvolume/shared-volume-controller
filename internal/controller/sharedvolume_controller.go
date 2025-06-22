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
	"math/rand"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	nfsv1alpha1 "github.com/sharedvolume/nfs-server-controller/api/v1alpha1"
	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SharedVolumeReconciler reconciles a SharedVolume object
type SharedVolumeReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=sharedvolumes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=sharedvolumes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=sv.sharedvolume.io,resources=sharedvolumes/finalizers,verbs=update

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
	_ = logf.FromContext(ctx)

	// TODO(user): your logic here

	// 1. Fetch the SharedVolume instance
	var sharedVolume svv1alpha1.SharedVolume
	err := r.Get(ctx, req.NamespacedName, &sharedVolume)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// 1.1 Set defaults
	sharedVolume.Spec.SetDefaults(sharedVolume.Namespace)
	if err := r.Update(ctx, &sharedVolume); err != nil {
		return ctrl.Result{}, err
	}

	// 1.2 Validate
	if err := sharedVolume.Spec.Validate(); err != nil {
		return ctrl.Result{}, err
	}

	// 2. If nfsServer is not defined, generate it
	if sharedVolume.Spec.NfsServer == nil {
		// Generate random 5-character alphanumeric string
		randomStr := RandString(5)
		name := sharedVolume.Name + "-" + randomStr
		namespace := sharedVolume.Namespace

		// Prepare NfsServer CR
		replicas := int32(1)
		nfsServer := &nfsv1alpha1.NfsServer{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: nfsv1alpha1.NfsServerSpec{
				Storage: nfsv1alpha1.StorageSpec{
					Capacity:         sharedVolume.Spec.Storage.Capacity,
					StorageClassName: sharedVolume.Spec.StorageClassName,
				},
				Replicas: &replicas,
				// Path, Image, PersistentVolume can be set here if needed
			},
		}
		// Create the NfsServer CR
		err = r.Client.Create(ctx, nfsServer)
		if err != nil {
			return ctrl.Result{}, err
		}
		// Set nfsServer spec in SharedVolume
		sharedVolume.Spec.NfsServer = &svv1alpha1.NfsServerSpec{
			Type:      "object",
			Name:      name,
			Namespace: namespace,
			// URL: should be set after NfsServer is ready (can be updated in a later reconcile)
		}
		if err := r.Update(ctx, &sharedVolume); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	return ctrl.Result{}, nil
}

// RandString generates a random alphanumeric string of given length.
func RandString(n int) string {
	letters := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")
	rand.Seed(time.Now().UnixNano())
	b := make([]rune, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// SetupWithManager sets up the controller with the Manager.
func (r *SharedVolumeReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&svv1alpha1.SharedVolume{}).
		Named("sharedvolume").
		Complete(r)
}
