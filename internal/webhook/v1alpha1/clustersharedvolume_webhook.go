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
var clustersharedvolumelog = logf.Log.WithName("clustersharedvolume-resource")

// SetupClusterSharedVolumeWebhookWithManager registers the ClusterSharedVolume webhook with the manager
func SetupClusterSharedVolumeWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).For(&svv1alpha1.ClusterSharedVolume{}).
		WithValidator(&ClusterSharedVolumeCustomValidator{}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-sv-sharedvolume-io-v1alpha1-clustersharedvolume,mutating=false,failurePolicy=fail,sideEffects=None,groups=sv.sharedvolume.io,resources=clustersharedvolumes,verbs=create;update,versions=v1alpha1,name=vclustersharedvolume.kb.io,admissionReviewVersions=v1

// ClusterSharedVolumeCustomValidator struct is responsible for validating the ClusterSharedVolume resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
// +kubebuilder:object:generate=false
type ClusterSharedVolumeCustomValidator struct{}

var _ webhook.CustomValidator = &ClusterSharedVolumeCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type ClusterSharedVolume.
func (v *ClusterSharedVolumeCustomValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	clustersharedvolume, ok := obj.(*svv1alpha1.ClusterSharedVolume)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterSharedVolume object but got %T", obj)
	}
	clustersharedvolumelog.Info("Validation for ClusterSharedVolume upon creation", "name", clustersharedvolume.GetName())

	return nil, validation.ValidateVolumeObject(clustersharedvolume, &clustersharedvolume.Spec.VolumeSpecBase, "ClusterSharedVolume")
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type ClusterSharedVolume.
func (v *ClusterSharedVolumeCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldClusterSharedVolume, ok := oldObj.(*svv1alpha1.ClusterSharedVolume)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterSharedVolume object for the oldObj but got %T", oldObj)
	}
	newClusterSharedVolume, ok := newObj.(*svv1alpha1.ClusterSharedVolume)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterSharedVolume object for the newObj but got %T", newObj)
	}
	clustersharedvolumelog.Info("Validation for ClusterSharedVolume upon update", "name", newClusterSharedVolume.GetName())

	return nil, validation.ValidateVolumeObjectUpdate(oldClusterSharedVolume, newClusterSharedVolume, &oldClusterSharedVolume.Spec.VolumeSpecBase, &newClusterSharedVolume.Spec.VolumeSpecBase, "ClusterSharedVolume")
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type ClusterSharedVolume.
func (v *ClusterSharedVolumeCustomValidator) ValidateDelete(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	clustersharedvolume, ok := obj.(*svv1alpha1.ClusterSharedVolume)
	if !ok {
		return nil, fmt.Errorf("expected a ClusterSharedVolume object but got %T", obj)
	}
	clustersharedvolumelog.Info("Validation for ClusterSharedVolume upon deletion", "name", clustersharedvolume.GetName())

	// No validation needed on deletion
	return nil, nil
}
