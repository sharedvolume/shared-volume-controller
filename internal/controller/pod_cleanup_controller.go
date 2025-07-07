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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/cleanup/base"
)

// PodCleanupReconciler reconciles Pod deletions for SharedVolume cleanup using the modular system
type PodCleanupReconciler struct {
	client.Client
	Scheme            *runtime.Scheme
	CleanupController *base.CleanupController
}

// NewPodCleanupReconciler creates a new pod cleanup reconciler with the modular cleanup controller
func NewPodCleanupReconciler(client client.Client, scheme *runtime.Scheme) *PodCleanupReconciler {
	return &PodCleanupReconciler{
		Client:            client,
		Scheme:            scheme,
		CleanupController: base.NewCleanupController(client, scheme),
	}
}

// Reconcile delegates to the modular cleanup controller
func (r *PodCleanupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	return r.CleanupController.Reconcile(ctx, req)
}

// SetupWithManager sets up the controller with the Manager
func (r *PodCleanupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.CleanupController.SetupWithManager(mgr)
}
