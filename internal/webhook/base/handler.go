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
	"encoding/json"
	"net/http"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/sharedvolume/shared-volume-controller/internal/webhook/processors"
	"github.com/sharedvolume/shared-volume-controller/internal/webhook/types"
	"github.com/sharedvolume/shared-volume-controller/internal/webhook/validators"
)

// WebhookHandler is the base handler for pod webhook operations
type WebhookHandler struct {
	Client              client.Client
	AnnotationProcessor *processors.AnnotationProcessor
	FinalizerProcessor  *processors.FinalizerProcessor
	VolumeProcessor     *processors.VolumeProcessor
	Validator           *validators.PodValidator
}

// NewWebhookHandler creates a new webhook handler with all processors
func NewWebhookHandler(client client.Client) *WebhookHandler {
	config := &types.WebhookConfig{
		Client: client,
	}

	return &WebhookHandler{
		Client:              client,
		AnnotationProcessor: processors.NewAnnotationProcessor(config),
		FinalizerProcessor:  processors.NewFinalizerProcessor(config),
		VolumeProcessor:     processors.NewVolumeProcessor(client, config),
		Validator:           validators.NewPodValidator(),
	}
}

// Handle processes the admission request
func (h *WebhookHandler) Handle(ctx context.Context, req admission.Request) admission.Response {
	logger := log.FromContext(ctx).WithName("pod-webhook")

	pod := &corev1.Pod{}
	if err := json.Unmarshal(req.Object.Raw, pod); err != nil {
		logger.Error(err, "failed to unmarshal pod")
		return admission.Errored(http.StatusBadRequest, err)
	}

	logger.Info("Processing pod", "name", pod.Name, "namespace", pod.Namespace, "operation", req.Operation)

	// Handle pod deletion
	if pod.DeletionTimestamp != nil {
		logger.Info("Pod is being deleted, checking for cleanup", "name", pod.Name, "namespace", pod.Namespace)

		if h.FinalizerProcessor.HasFinalizer(pod, types.SharedVolumePodFinalizer) {
			if err := h.handlePodDeletion(ctx, pod); err != nil {
				logger.Error(err, "failed to handle pod deletion cleanup", "name", pod.Name, "namespace", pod.Namespace)
				return admission.Errored(http.StatusInternalServerError, err)
			}
		}

		return admission.Allowed("Pod deletion processed")
	}

	// Skip processing for update operations if already processed
	if req.Operation == "UPDATE" && h.shouldSkipUpdate(pod) {
		logger.Info("Pod already processed, skipping", "name", pod.Name, "namespace", pod.Namespace)
		return admission.Allowed("Pod already processed")
	}

	// Extract shared volume annotations
	sharedVolumes := h.AnnotationProcessor.ExtractVolumeReferences(pod)
	if len(sharedVolumes) == 0 {
		return admission.Allowed("No shared volume annotations found")
	}

	// Validate pod for volume processing
	if err := h.Validator.ValidateForVolumeProcessing(pod, sharedVolumes); err != nil {
		logger.Error(err, "pod validation failed")
		return admission.Errored(http.StatusBadRequest, err)
	}

	// Process each shared volume
	for _, svRef := range sharedVolumes {
		if err := h.VolumeProcessor.ProcessSharedVolume(ctx, pod, svRef); err != nil {
			logger.Error(err, "failed to process shared volume", "sharedVolume", svRef.Name, "namespace", svRef.Namespace)
			return admission.Errored(http.StatusInternalServerError, err)
		}
	}

	// Add finalizer to ensure cleanup when pod is deleted
	h.FinalizerProcessor.AddFinalizer(pod, types.SharedVolumePodFinalizer)

	// Marshal the modified pod
	marshaledPod, err := json.Marshal(pod)
	if err != nil {
		logger.Error(err, "failed to marshal pod")
		return admission.Errored(http.StatusInternalServerError, err)
	}

	return admission.PatchResponseFromRaw(req.Object.Raw, marshaledPod)
}

// shouldSkipUpdate determines if an update operation should be skipped
func (h *WebhookHandler) shouldSkipUpdate(pod *corev1.Pod) bool {
	return h.FinalizerProcessor.HasFinalizer(pod, types.SharedVolumePodFinalizer) && len(pod.Spec.Volumes) > 0
}

// handlePodDeletion handles cleanup when a pod with shared volumes is being deleted
func (h *WebhookHandler) handlePodDeletion(ctx context.Context, pod *corev1.Pod) error {
	logger := log.FromContext(ctx).WithName("pod-deletion-handler")

	// Extract shared volume references from the pod annotations
	sharedVolumes := h.AnnotationProcessor.ExtractVolumeReferences(pod)
	if len(sharedVolumes) == 0 {
		logger.Info("No shared volumes found in pod, removing finalizer", "pod", pod.Name)
		return h.removePodFinalizer(ctx, pod)
	}

	// For each shared volume, check if any other pods are still using it
	for _, svRef := range sharedVolumes {
		if err := h.VolumeProcessor.CleanupVolumeIfUnused(ctx, svRef, pod); err != nil {
			logger.Error(err, "failed to cleanup shared volume", "volume", svRef.Name, "pod", pod.Name)
			return err
		}
	}

	// Remove finalizer from pod after successful cleanup
	return h.removePodFinalizer(ctx, pod)
}

// removePodFinalizer removes the shared volume finalizer from the pod
func (h *WebhookHandler) removePodFinalizer(ctx context.Context, pod *corev1.Pod) error {
	h.FinalizerProcessor.RemoveFinalizer(pod, types.SharedVolumePodFinalizer)

	// Update the pod to remove the finalizer
	if err := h.Client.Update(ctx, pod); err != nil {
		return err
	}

	return nil
}
