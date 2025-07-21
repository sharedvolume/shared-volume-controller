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

package v1alpha1

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
	"github.com/sharedvolume/shared-volume-controller/internal/webhook/validation"
)

// log is for logging in this package.
var sharedvolumelog = logf.Log.WithName("sharedvolume-resource")

// SetupSharedVolumeWebhookWithManager registers the SharedVolume webhook with the manager
func SetupSharedVolumeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&svv1alpha1.SharedVolume{}).
		WithValidator(&SharedVolumeCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-sv-sharedvolume-io-v1alpha1-sharedvolume,mutating=false,failurePolicy=fail,sideEffects=None,groups=sv.sharedvolume.io,resources=sharedvolumes,verbs=create;update,versions=v1alpha1,name=vsharedvolume.kb.io,admissionReviewVersions=v1

// SharedVolumeCustomValidator struct is responsible for validating the SharedVolume resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type SharedVolumeCustomValidator struct{}

var _ webhook.CustomValidator = &SharedVolumeCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type SharedVolume.
func (v *SharedVolumeCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	sharedvolume, ok := obj.(*svv1alpha1.SharedVolume)
	if !ok {
		return nil, fmt.Errorf("expected a SharedVolume object but got %T", obj)
	}
	sharedvolumelog.Info("Validation for SharedVolume upon creation", "name", sharedvolume.GetName())

	return nil, validation.ValidateVolumeObject(sharedvolume, &sharedvolume.Spec.VolumeSpecBase, "SharedVolume")
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type SharedVolume.
func (v *SharedVolumeCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	sharedvolume, ok := newObj.(*svv1alpha1.SharedVolume)
	if !ok {
		return nil, fmt.Errorf("expected a SharedVolume object for the newObj but got %T", newObj)
	}
	sharedvolumelog.Info("Validation for SharedVolume upon update", "name", sharedvolume.GetName())

	return nil, validation.ValidateVolumeObject(sharedvolume, &sharedvolume.Spec.VolumeSpecBase, "SharedVolume")
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type SharedVolume.
func (v *SharedVolumeCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	sharedvolume, ok := obj.(*svv1alpha1.SharedVolume)
	if !ok {
		return nil, fmt.Errorf("expected a SharedVolume object but got %T", obj)
	}
	sharedvolumelog.Info("Validation for SharedVolume upon deletion", "name", sharedvolume.GetName())

	// No validation needed on deletion
	return nil, nil
}
