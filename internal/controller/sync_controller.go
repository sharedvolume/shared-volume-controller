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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

const (
	SyncControllerName = "sync-controller"
)

// SyncRequest represents the payload for sync API calls
type SyncRequest struct {
	Source  SyncSource `json:"source"`
	Target  SyncTarget `json:"target"`
	Timeout string     `json:"timeout"`
}

type SyncSource struct {
	Type    string        `json:"type"`
	Details SyncSourceSSH `json:"details"`
}

type SyncSourceSSH struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Path       string `json:"path"`
	PrivateKey string `json:"privateKey"`
}

type SyncTarget struct {
	Path string `json:"path"`
}

// SyncController manages sync operations for SharedVolumes
type SyncController struct {
	client.Client
	Scheme          *runtime.Scheme
	syncTimers      map[string]*time.Timer
	initialSyncDone map[string]bool // Track which SharedVolumes have had their initial sync
	mutex           sync.RWMutex
	httpClient      *http.Client
}

// NewSyncController creates a new SyncController
func NewSyncController(client client.Client, scheme *runtime.Scheme) *SyncController {
	return &SyncController{
		Client:          client,
		Scheme:          scheme,
		syncTimers:      make(map[string]*time.Timer),
		initialSyncDone: make(map[string]bool),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// StartSyncForSharedVolume starts sync operations for a SharedVolume
func (s *SyncController) StartSyncForSharedVolume(ctx context.Context, sv *svv1alpha1.SharedVolume) error {
	log := logf.FromContext(ctx).WithName(SyncControllerName)

	// Check if SharedVolume is ready and has source configured
	if sv.Status.Phase != "Ready" || sv.Spec.Source == nil || sv.Spec.Source.SSH == nil {
		log.Info("SharedVolume not ready for sync or source not configured",
			"name", sv.Name, "namespace", sv.Namespace, "phase", sv.Status.Phase)
		return nil
	}

	key := fmt.Sprintf("%s/%s", sv.Namespace, sv.Name)

	// Check if sync is already running for this SharedVolume
	s.mutex.RLock()
	_, alreadyRunning := s.syncTimers[key]
	initialSyncDone := s.initialSyncDone[key]
	s.mutex.RUnlock()

	if alreadyRunning {
		log.Info("Sync already running for SharedVolume", "name", sv.Name, "namespace", sv.Namespace)
		return nil
	}

	// Stop existing timer if any (safety check)
	s.stopSyncTimer(key)

	// Parse sync interval
	interval, err := time.ParseDuration(sv.Spec.SyncInterval)
	if err != nil {
		log.Error(err, "Failed to parse sync interval", "interval", sv.Spec.SyncInterval)
		return err
	}

	log.Info("Starting sync operations for SharedVolume",
		"name", sv.Name, "namespace", sv.Namespace, "interval", interval, "initialSyncDone", initialSyncDone)

	// For recovery scenarios, we don't trigger initial sync immediately
	// Instead, we wait for the first interval to pass, then start syncing
	// This prevents thundering herd problems on controller restart
	if !initialSyncDone {
		// Mark initial sync as done to prevent duplicate initial syncs
		s.mutex.Lock()
		s.initialSyncDone[key] = true
		s.mutex.Unlock()
	}

	// Schedule periodic sync (first sync will happen after the interval)
	timer := time.AfterFunc(interval, func() {
		s.scheduledSync(sv.DeepCopy(), interval)
	})

	s.mutex.Lock()
	s.syncTimers[key] = timer
	s.mutex.Unlock()

	log.Info("Started sync for SharedVolume",
		"name", sv.Name, "namespace", sv.Namespace, "interval", interval)

	return nil
}

// StopSyncForSharedVolume stops sync operations for a SharedVolume
func (s *SyncController) StopSyncForSharedVolume(sv *svv1alpha1.SharedVolume) {
	key := fmt.Sprintf("%s/%s", sv.Namespace, sv.Name)
	s.stopSyncTimer(key)
}

// stopSyncTimer stops and removes a sync timer
func (s *SyncController) stopSyncTimer(key string) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if timer, exists := s.syncTimers[key]; exists {
		timer.Stop()
		delete(s.syncTimers, key)
	}
	// Also clean up the initial sync tracking when stopping
	delete(s.initialSyncDone, key)
}

// scheduledSync handles periodic sync execution
func (s *SyncController) scheduledSync(sv *svv1alpha1.SharedVolume, interval time.Duration) {
	ctx := context.Background()
	log := logf.FromContext(ctx).WithName(SyncControllerName)
	key := fmt.Sprintf("%s/%s", sv.Namespace, sv.Name)

	// Always schedule the next sync first, regardless of any errors that might occur
	defer func() {
		timer := time.AfterFunc(interval, func() {
			s.scheduledSync(sv.DeepCopy(), interval)
		})

		s.mutex.Lock()
		s.syncTimers[key] = timer
		s.mutex.Unlock()

		log.Info("Scheduled next sync",
			"name", sv.Name, "namespace", sv.Namespace, "interval", interval)
	}()

	// Get the latest version of the SharedVolume
	var latestSV svv1alpha1.SharedVolume
	err := s.Get(ctx, types.NamespacedName{
		Name:      sv.Name,
		Namespace: sv.Namespace,
	}, &latestSV)

	if err != nil {
		log.Error(err, "Failed to get latest SharedVolume for sync, will retry at next interval",
			"name", sv.Name, "namespace", sv.Namespace)
		return
	}

	// Check if SharedVolume is still ready and has source configured
	if latestSV.Status.Phase != "Ready" || latestSV.Spec.Source == nil || latestSV.Spec.Source.SSH == nil {
		log.Info("SharedVolume no longer ready for sync, will check again at next interval",
			"name", sv.Name, "namespace", sv.Namespace, "phase", latestSV.Status.Phase)
		return
	}

	// Trigger sync - errors are logged but don't stop the scheduling
	if err := s.triggerSync(ctx, &latestSV); err != nil {
		log.Error(err, "Failed to trigger scheduled sync, will retry at next interval",
			"name", sv.Name, "namespace", sv.Namespace)
	} else {
		log.Info("Successfully completed scheduled sync",
			"name", sv.Name, "namespace", sv.Namespace)
	}
}

// triggerSync performs the actual sync API call
func (s *SyncController) triggerSync(ctx context.Context, sv *svv1alpha1.SharedVolume) error {
	log := logf.FromContext(ctx).WithName(SyncControllerName)

	// Build sync request payload
	syncReq, err := s.buildSyncRequest(ctx, sv)
	if err != nil {
		return fmt.Errorf("failed to build sync request: %w", err)
	}

	// Marshal to JSON
	payload, err := json.Marshal(syncReq)
	if err != nil {
		return fmt.Errorf("failed to marshal sync request: %w", err)
	}

	// Log the exact payload being sent for debugging
	log.Info("Sending sync request",
		"url", fmt.Sprintf("http://%s.%s.svc.cluster.local:8080/api/1.0/sync", sv.Spec.ReferenceValue, sv.Namespace),
		"payload", string(payload),
		"targetPath", syncReq.Target.Path,
		"sourceHost", syncReq.Source.Details.Host,
		"sourceUsername", syncReq.Source.Details.Username,
		"sourcePath", syncReq.Source.Details.Path,
		"timeout", syncReq.Timeout,
		"privateKeyPrefix", syncReq.Source.Details.PrivateKey[:50]+"...")

	// Build URL using Kubernetes service DNS format
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local:8080/api/1.0/sync", sv.Spec.ReferenceValue, sv.Namespace)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set required headers to match the working manual request
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(payload)))
	// Don't set Host header manually - let Go handle it automatically

	// Execute request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute sync request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("sync request failed with status %d", resp.StatusCode)
	}

	log.Info("Successfully triggered sync",
		"name", sv.Name, "namespace", sv.Namespace, "url", url)

	return nil
}

// buildSyncRequest builds the sync request payload from SharedVolume spec
func (s *SyncController) buildSyncRequest(ctx context.Context, sv *svv1alpha1.SharedVolume) (*SyncRequest, error) {
	if sv.Spec.Source == nil || sv.Spec.Source.SSH == nil {
		return nil, fmt.Errorf("SSH source not configured")
	}

	ssh := sv.Spec.Source.SSH

	// Get private key (could be direct or from secret)
	privateKey := ssh.PrivateKey
	if privateKey == "" && ssh.PrivateKeyFromSecret != nil {
		// Read private key from secret
		var err error
		privateKey, err = s.getPrivateKeyFromSecret(ctx, sv.Namespace, ssh.PrivateKeyFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key from secret: %w", err)
		}
		// If from secret, the secret data is the actual key content (not base64)
		// We need to base64 encode it for the API
		privateKey = base64.StdEncoding.EncodeToString([]byte(privateKey))
	}
	// If privateKey comes directly from YAML spec, it's already base64 encoded, so use as-is
	// Ensure we have a valid base64 private key
	if privateKey == "" {
		return nil, fmt.Errorf("private key not provided")
	}

	// Set default port if not specified
	port := ssh.Port
	if port == 0 {
		port = 22
	}

	// Set default timeout if not specified
	timeout := sv.Spec.SyncTimeout
	if timeout == "" {
		timeout = "120s"
	}

	// Build target path: /nfs/{sv.Name}-{sv.Namespace}
	targetPath := fmt.Sprintf("/nfs/%s-%s", sv.Name, sv.Namespace)

	return &SyncRequest{
		Source: SyncSource{
			Type: "ssh",
			Details: SyncSourceSSH{
				Host:       ssh.Host,
				Port:       port,
				Username:   ssh.Username,
				Path:       ssh.Path,
				PrivateKey: privateKey,
			},
		},
		Target: SyncTarget{
			Path: targetPath,
		},
		Timeout: timeout,
	}, nil
}

// getPrivateKeyFromSecret reads a private key from a Kubernetes secret
func (s *SyncController) getPrivateKeyFromSecret(ctx context.Context, namespace string, selector *svv1alpha1.SecretKeySelector) (string, error) {
	var secret v1.Secret
	err := s.Get(ctx, types.NamespacedName{
		Name:      selector.Name,
		Namespace: namespace,
	}, &secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, selector.Name, err)
	}

	privateKeyBytes, exists := secret.Data[selector.Key]
	if !exists {
		return "", fmt.Errorf("key %s not found in secret %s/%s", selector.Key, namespace, selector.Name)
	}

	return string(privateKeyBytes), nil
}

// RecoverSyncOperations starts sync operations for all existing ready SharedVolumes
// This should be called when the controller starts to resume sync operations after pod restart
func (s *SyncController) RecoverSyncOperations(ctx context.Context) error {
	log := logf.FromContext(ctx).WithName(SyncControllerName)

	// List all SharedVolumes across all namespaces
	var sharedVolumeList svv1alpha1.SharedVolumeList
	if err := s.List(ctx, &sharedVolumeList); err != nil {
		return fmt.Errorf("failed to list SharedVolumes for sync recovery: %w", err)
	}

	recoveredCount := 0
	for _, sv := range sharedVolumeList.Items {
		// Skip if not ready or no source configured
		if sv.Status.Phase != "Ready" || sv.Spec.Source == nil || sv.Spec.Source.SSH == nil {
			continue
		}

		// Start sync operations for this SharedVolume
		if err := s.StartSyncForSharedVolume(ctx, &sv); err != nil {
			log.Error(err, "Failed to recover sync for SharedVolume",
				"name", sv.Name, "namespace", sv.Namespace)
			// Continue with other SharedVolumes
			continue
		}

		recoveredCount++
		log.Info("Recovered sync operations for SharedVolume",
			"name", sv.Name, "namespace", sv.Namespace)
	}

	log.Info("Completed sync recovery", "recoveredCount", recoveredCount, "totalSharedVolumes", len(sharedVolumeList.Items))
	return nil
}
