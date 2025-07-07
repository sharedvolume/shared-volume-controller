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

package v1

import (
	"context"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/sharedvolume/shared-volume-controller/internal/webhook/base"
)

// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=fail,sideEffects=None,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod.kb.io,admissionReviewVersions=v1

// PodAnnotator annotates Pods using the modular webhook system
type PodAnnotator struct {
	Client  client.Client
	handler *base.WebhookHandler
}

// NewPodAnnotator creates a new pod annotator with the modular webhook handler
func NewPodAnnotator(client client.Client) *PodAnnotator {
	return &PodAnnotator{
		Client:  client,
		handler: base.NewWebhookHandler(client),
	}
}

// Handle processes the admission request using the modular webhook handler
func (a *PodAnnotator) Handle(ctx context.Context, req admission.Request) admission.Response {
	return a.handler.Handle(ctx, req)
}
