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
	"io"
	"net/http"
	"sync"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
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
	Type    string      `json:"type"`
	Details interface{} `json:"details"`
}

type SyncSourceSSH struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	User       string `json:"user"`
	Path       string `json:"path"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
}

type SyncSourceHTTP struct {
	URL string `json:"url"`
}

type SyncSourceGit struct {
	URL        string `json:"url"`
	User       string `json:"user,omitempty"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
	Branch     string `json:"branch,omitempty"`
}

type SyncSourceS3 struct {
	EndpointURL string `json:"endpointUrl"`
	BucketName  string `json:"bucketName"`
	Path        string `json:"path,omitempty"`
	AccessKey   string `json:"accessKey"`
	SecretKey   string `json:"secretKey"`
	Region      string `json:"region"`
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

// hasValidSource checks if the SharedVolume has any valid source configured
func (s *SyncController) hasValidSource(sv *svv1alpha1.SharedVolume) bool {
	if sv.Spec.Source == nil {
		return false
	}
	return sv.Spec.Source.SSH != nil || sv.Spec.Source.HTTP != nil || sv.Spec.Source.Git != nil || sv.Spec.Source.S3 != nil
}

// StartSyncForSharedVolume starts sync operations for a SharedVolume
func (s *SyncController) StartSyncForSharedVolume(ctx context.Context, sv *svv1alpha1.SharedVolume) error {
	log := logf.FromContext(ctx).WithName(SyncControllerName)

	// Check if SharedVolume is ready and has source configured
	if sv.Status.Phase != "Ready" || !s.hasValidSource(sv) {
		log.Info("SharedVolume not ready for sync or source not configured",
			"name", sv.Name, "namespace", sv.Namespace, "phase", sv.Status.Phase)
		// Stop any existing sync operations if not ready or source not configured
		s.StopSyncForSharedVolume(sv)
		return nil
	}

	// Always stop existing sync operations first to ensure configuration changes are picked up
	s.StopSyncForSharedVolume(sv)

	log.Info("Restarting sync operations with updated configuration", "name", sv.Name, "namespace", sv.Namespace)

	// Parse sync interval
	interval, err := time.ParseDuration(sv.Spec.SyncInterval)
	if err != nil {
		log.Error(err, "Failed to parse sync interval", "interval", sv.Spec.SyncInterval)
		return err
	}

	// Start sync operations with the updated configuration
	return s.startSyncCommon(sv.Name, sv.Namespace, sv.Spec.VolumeSpecBase, interval)
}

// StopSyncForSharedVolume stops sync operations for a SharedVolume
func (s *SyncController) StopSyncForSharedVolume(sv *svv1alpha1.SharedVolume) {
	key := fmt.Sprintf("%s/%s", sv.Namespace, sv.Name)
	s.stopSyncTimer(key)
}

// StartSyncForClusterSharedVolume starts sync operations for a ClusterSharedVolume
func (s *SyncController) StartSyncForClusterSharedVolume(ctx context.Context, csv *svv1alpha1.ClusterSharedVolume) error {
	log := logf.FromContext(ctx).WithName(SyncControllerName)

	// Check if sync is enabled and source is configured
	if !s.hasValidSourceCSV(csv) {
		log.Info("No valid source configured for ClusterSharedVolume, skipping sync", "name", csv.Name)
		// Stop any existing sync operations if source is no longer configured
		s.StopSyncForClusterSharedVolume(csv)
		return nil
	}

	// Parse sync interval
	interval, err := time.ParseDuration(csv.Spec.SyncInterval)
	if err != nil {
		return fmt.Errorf("invalid sync interval: %w", err)
	}

	// Always stop existing sync operations first to ensure configuration changes are picked up
	s.StopSyncForClusterSharedVolume(csv)

	log.Info("Restarting sync operations with updated configuration", "name", csv.Name, "namespace", csv.Spec.ResourceNamespace)

	// Start sync operations with the updated configuration
	return s.startSyncCommon(csv.Name, csv.Spec.ResourceNamespace, csv.Spec.VolumeSpecBase, interval)
}

// StopSyncForClusterSharedVolume stops sync operations for a ClusterSharedVolume
func (s *SyncController) StopSyncForClusterSharedVolume(csv *svv1alpha1.ClusterSharedVolume) {
	key := fmt.Sprintf("%s/%s", csv.Spec.ResourceNamespace, csv.Name)
	s.stopSyncTimer(key)
}

// hasValidSourceCSV checks if ClusterSharedVolume has a valid source configuration
func (s *SyncController) hasValidSourceCSV(csv *svv1alpha1.ClusterSharedVolume) bool {
	if csv.Spec.Source == nil {
		return false
	}
	// Check if at least one source type is configured
	return csv.Spec.Source.SSH != nil || csv.Spec.Source.HTTP != nil || csv.Spec.Source.Git != nil || csv.Spec.Source.S3 != nil
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

// startSyncCommon handles the common logic for starting sync operations
func (s *SyncController) startSyncCommon(name, namespace string, spec svv1alpha1.VolumeSpecBase, interval time.Duration) error {
	log := logf.FromContext(context.Background()).WithName(SyncControllerName)
	key := fmt.Sprintf("%s/%s", namespace, name)

	// Check if sync is already running
	s.mutex.RLock()
	_, alreadyRunning := s.syncTimers[key]
	initialSyncDone := s.initialSyncDone[key]
	s.mutex.RUnlock()

	if alreadyRunning {
		log.Info("Sync already running for volume", "name", name, "namespace", namespace)
		return nil
	}

	// Stop existing timer if any (safety check)
	s.stopSyncTimer(key)

	log.Info("Starting sync operations for volume",
		"name", name, "namespace", namespace, "interval", interval, "initialSyncDone", initialSyncDone)

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
		s.scheduledSyncCommon(name, namespace, spec, interval)
	})

	s.mutex.Lock()
	s.syncTimers[key] = timer
	s.mutex.Unlock()

	log.Info("Started sync for volume",
		"name", name, "namespace", namespace, "interval", interval)

	return nil
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
	if latestSV.Status.Phase != "Ready" || !s.hasValidSource(&latestSV) {
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

// scheduledSyncCommon handles periodic sync execution for any volume type
func (s *SyncController) scheduledSyncCommon(name, namespace string, spec svv1alpha1.VolumeSpecBase, interval time.Duration) {
	ctx := context.Background()
	log := logf.FromContext(ctx).WithName(SyncControllerName)
	key := fmt.Sprintf("%s/%s", namespace, name)

	// Check if the volume still exists before scheduling next sync
	// Try SharedVolume first, then ClusterSharedVolume
	var volumeExists bool

	// Check for SharedVolume
	var sv svv1alpha1.SharedVolume
	err := s.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, &sv)
	if err == nil {
		volumeExists = true
	} else if apierrors.IsNotFound(err) {
		// Check for ClusterSharedVolume
		var csv svv1alpha1.ClusterSharedVolume
		err = s.Get(ctx, types.NamespacedName{Name: name}, &csv)
		if err == nil {
			volumeExists = true
		} else if apierrors.IsNotFound(err) {
			volumeExists = false
		} else {
			log.Error(err, "Failed to check if ClusterSharedVolume exists", "name", name)
			volumeExists = false
		}
	} else {
		log.Error(err, "Failed to check if SharedVolume exists", "name", name, "namespace", namespace)
		volumeExists = false
	}

	// If volume doesn't exist, stop sync operations and don't reschedule
	if !volumeExists {
		log.Info("Volume no longer exists, stopping sync operations", "name", name, "namespace", namespace)
		s.stopSyncTimer(key)
		return
	}

	// Always schedule the next sync first, regardless of any errors that might occur
	defer func() {
		timer := time.AfterFunc(interval, func() {
			s.scheduledSyncCommon(name, namespace, spec, interval)
		})

		s.mutex.Lock()
		s.syncTimers[key] = timer
		s.mutex.Unlock()

		log.Info("Scheduled next sync",
			"name", name, "namespace", namespace, "interval", interval)
	}()

	// Check if we have a valid source configured
	if spec.Source == nil {
		log.Info("No source configured for volume, skipping sync", "name", name, "namespace", namespace)
		return
	}

	// Check if at least one source type is configured
	if spec.Source.SSH == nil && spec.Source.HTTP == nil && spec.Source.Git == nil && spec.Source.S3 == nil {
		log.Info("No valid source type configured for volume, skipping sync", "name", name, "namespace", namespace)
		return
	}

	// Trigger sync - errors are logged but don't stop the scheduling
	if err := s.triggerSyncCommon(ctx, name, namespace, spec); err != nil {
		log.Error(err, "Failed to trigger scheduled sync, will retry at next interval",
			"name", name, "namespace", namespace)
	} else {
		log.Info("Successfully completed scheduled sync",
			"name", name, "namespace", namespace)
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
		"sourceType", syncReq.Source.Type,
		"timeout", syncReq.Timeout)

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

// triggerSyncCommon performs the actual sync API call for any volume type
func (s *SyncController) triggerSyncCommon(ctx context.Context, name, namespace string, spec svv1alpha1.VolumeSpecBase) error {
	log := logf.FromContext(ctx).WithName(SyncControllerName)

	// Build sync request payload
	syncReq, err := s.buildSyncRequestCommon(ctx, name, namespace, spec)
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
		"url", fmt.Sprintf("http://%s.%s.svc.cluster.local:8080/api/1.0/sync", spec.ReferenceValue, namespace),
		"payload", string(payload),
		"targetPath", syncReq.Target.Path,
		"sourceType", syncReq.Source.Type,
		"timeout", syncReq.Timeout)

	// Build URL using Kubernetes service DNS format
	url := fmt.Sprintf("http://%s.%s.svc.cluster.local:8080/api/1.0/sync", spec.ReferenceValue, namespace)

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

	// Check response status - accept any 2xx status code
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("sync request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	log.Info("Sync request completed successfully",
		"name", name, "namespace", namespace, "status", resp.StatusCode)

	return nil
}

// buildSyncRequest builds the sync request payload from SharedVolume spec
func (s *SyncController) buildSyncRequest(ctx context.Context, sv *svv1alpha1.SharedVolume) (*SyncRequest, error) {
	if sv.Spec.Source == nil {
		return nil, fmt.Errorf("no source configured")
	}

	// Set default timeout if not specified
	timeout := sv.Spec.SyncTimeout
	if timeout == "" {
		timeout = "120s"
	}

	// Build target path: /nfs/{sv.Name}-{sv.Namespace}
	targetPath := fmt.Sprintf("/nfs/%s-%s", sv.Name, sv.Namespace)

	// Handle different source types
	if sv.Spec.Source.SSH != nil {
		return s.buildSSHSyncRequest(ctx, sv, timeout, targetPath)
	} else if sv.Spec.Source.HTTP != nil {
		return s.buildHTTPSyncRequest(sv, timeout, targetPath)
	} else if sv.Spec.Source.Git != nil {
		return s.buildGitSyncRequest(ctx, sv, timeout, targetPath)
	} else if sv.Spec.Source.S3 != nil {
		return s.buildS3SyncRequest(ctx, sv, timeout, targetPath)
	}

	return nil, fmt.Errorf("no valid source type configured")
}

// buildSSHSyncRequest builds sync request for SSH source
func (s *SyncController) buildSSHSyncRequest(ctx context.Context, sv *svv1alpha1.SharedVolume, timeout, targetPath string) (*SyncRequest, error) {
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

	// Get password (could be direct or from secret)
	password := ssh.Password
	if password == "" && ssh.PasswordFromSecret != nil {
		var err error
		// For SharedVolume, always use the SharedVolume's namespace for secrets
		password, err = s.getSecretValue(ctx, sv.Namespace, ssh.PasswordFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read password from secret: %w", err)
		}
	}

	// Ensure we have either private key or password for authentication
	if privateKey == "" && password == "" {
		return nil, fmt.Errorf("either private key or password must be provided for SSH authentication")
	}

	// Set default port if not specified
	port := ssh.Port
	if port == 0 {
		port = 22
	}

	return &SyncRequest{
		Source: SyncSource{
			Type: "ssh",
			Details: SyncSourceSSH{
				Host:       ssh.Host,
				Port:       port,
				User:       ssh.User,
				Path:       ssh.Path,
				Password:   password,
				PrivateKey: privateKey,
			},
		},
		Target: SyncTarget{
			Path: targetPath,
		},
		Timeout: timeout,
	}, nil
}

// buildHTTPSyncRequest builds sync request for HTTP source
func (s *SyncController) buildHTTPSyncRequest(sv *svv1alpha1.SharedVolume, timeout, targetPath string) (*SyncRequest, error) {
	http := sv.Spec.Source.HTTP

	return &SyncRequest{
		Source: SyncSource{
			Type: "http",
			Details: SyncSourceHTTP{
				URL: http.URL,
			},
		},
		Target: SyncTarget{
			Path: targetPath,
		},
		Timeout: timeout,
	}, nil
}

// buildGitSyncRequest builds sync request for Git source
func (s *SyncController) buildGitSyncRequest(ctx context.Context, sv *svv1alpha1.SharedVolume, timeout, targetPath string) (*SyncRequest, error) {
	git := sv.Spec.Source.Git

	// Get password (could be direct or from secret)
	password := git.Password
	if password == "" && git.PasswordFromSecret != nil {
		var err error
		// For SharedVolume, always use the SharedVolume's namespace for secrets
		password, err = s.getSecretValue(ctx, sv.Namespace, git.PasswordFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read password from secret: %w", err)
		}
	}

	// Get private key (could be direct or from secret)
	privateKey := git.PrivateKey
	if privateKey == "" && git.PrivateKeyFromSecret != nil {
		var err error
		// For SharedVolume, always use the SharedVolume's namespace for secrets
		privateKey, err = s.getPrivateKeyFromSecret(ctx, sv.Namespace, git.PrivateKeyFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key from secret: %w", err)
		}
		// If from secret, the secret data is the actual key content (not base64)
		// We need to base64 encode it for the API
		privateKey = base64.StdEncoding.EncodeToString([]byte(privateKey))
	}

	return &SyncRequest{
		Source: SyncSource{
			Type: "git",
			Details: SyncSourceGit{
				URL:        git.URL,
				User:       git.User,
				Password:   password,
				PrivateKey: privateKey,
				Branch:     git.Branch,
			},
		},
		Target: SyncTarget{
			Path: targetPath,
		},
		Timeout: timeout,
	}, nil
}

// buildS3SyncRequest builds sync request for S3 source
func (s *SyncController) buildS3SyncRequest(ctx context.Context, sv *svv1alpha1.SharedVolume, timeout, targetPath string) (*SyncRequest, error) {
	s3 := sv.Spec.Source.S3

	// Get access key (could be direct or from secret)
	accessKey := s3.AccessKey
	if accessKey == "" && s3.AccessKeyFromSecret != nil {
		var err error
		// For SharedVolume, always use the SharedVolume's namespace for secrets
		accessKey, err = s.getSecretValue(ctx, sv.Namespace, s3.AccessKeyFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read access key from secret: %w", err)
		}
	}

	// Get secret key (could be direct or from secret)
	secretKey := s3.SecretKey
	if secretKey == "" && s3.SecretKeyFromSecret != nil {
		var err error
		// For SharedVolume, always use the SharedVolume's namespace for secrets
		secretKey, err = s.getSecretValue(ctx, sv.Namespace, s3.SecretKeyFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read secret key from secret: %w", err)
		}
	}

	return &SyncRequest{
		Source: SyncSource{
			Type: "s3",
			Details: SyncSourceS3{
				EndpointURL: s3.EndpointURL,
				BucketName:  s3.BucketName,
				Path:        s3.Path,
				AccessKey:   accessKey,
				SecretKey:   secretKey,
				Region:      s3.Region,
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

// getSecretValue reads a value from a Kubernetes secret
func (s *SyncController) getSecretValue(ctx context.Context, namespace string, selector *svv1alpha1.SecretKeySelector) (string, error) {
	var secret v1.Secret
	err := s.Get(ctx, types.NamespacedName{
		Name:      selector.Name,
		Namespace: namespace,
	}, &secret)
	if err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, selector.Name, err)
	}

	valueBytes, exists := secret.Data[selector.Key]
	if !exists {
		return "", fmt.Errorf("key %s not found in secret %s/%s", selector.Key, namespace, selector.Name)
	}

	return string(valueBytes), nil
}

// RecoverSyncOperations starts sync operations for all existing ready SharedVolumes and ClusterSharedVolumes
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
		if sv.Status.Phase != "Ready" || !s.hasValidSource(&sv) {
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

	// List all ClusterSharedVolumes
	var clusterSharedVolumeList svv1alpha1.ClusterSharedVolumeList
	if err := s.List(ctx, &clusterSharedVolumeList); err != nil {
		log.Error(err, "Failed to list ClusterSharedVolumes for sync recovery")
		// Don't fail the entire recovery, continue with what we have
	} else {
		for _, csv := range clusterSharedVolumeList.Items {
			// Skip if not ready or no source configured
			if csv.Status.Phase != "Ready" || !s.hasValidSourceCSV(&csv) {
				continue
			}

			// Start sync operations for this ClusterSharedVolume
			if err := s.StartSyncForClusterSharedVolume(ctx, &csv); err != nil {
				log.Error(err, "Failed to recover sync for ClusterSharedVolume",
					"name", csv.Name)
				// Continue with other ClusterSharedVolumes
				continue
			}

			recoveredCount++
			log.Info("Recovered sync operations for ClusterSharedVolume",
				"name", csv.Name)
		}
	}

	log.Info("Completed sync recovery",
		"recoveredCount", recoveredCount,
		"totalSharedVolumes", len(sharedVolumeList.Items),
		"totalClusterSharedVolumes", len(clusterSharedVolumeList.Items))
	return nil
}

// buildSyncRequestCommon builds the sync request payload from VolumeSpecBase
func (s *SyncController) buildSyncRequestCommon(ctx context.Context, name, namespace string, spec svv1alpha1.VolumeSpecBase) (*SyncRequest, error) {
	if spec.Source == nil {
		return nil, fmt.Errorf("no source configured")
	}

	// Set default timeout if not specified
	timeout := spec.SyncTimeout
	if timeout == "" {
		timeout = "120s"
	}

	// Build target path: /nfs/{name}-{namespace}
	targetPath := fmt.Sprintf("/nfs/%s-%s", name, namespace)

	// Handle different source types
	if spec.Source.SSH != nil {
		return s.buildSSHSyncRequestCommon(ctx, name, namespace, spec, timeout, targetPath)
	} else if spec.Source.HTTP != nil {
		return s.buildHTTPSyncRequestCommon(spec, timeout, targetPath)
	} else if spec.Source.Git != nil {
		return s.buildGitSyncRequestCommon(ctx, name, namespace, spec, timeout, targetPath)
	} else if spec.Source.S3 != nil {
		return s.buildS3SyncRequestCommon(ctx, name, namespace, spec, timeout, targetPath)
	}

	return nil, fmt.Errorf("no valid source type configured")
}

// buildSSHSyncRequestCommon builds sync request for SSH source from VolumeSpecBase
func (s *SyncController) buildSSHSyncRequestCommon(ctx context.Context, name, namespace string, spec svv1alpha1.VolumeSpecBase, timeout, targetPath string) (*SyncRequest, error) {
	ssh := spec.Source.SSH

	// Get private key (could be direct or from secret)
	privateKey := ssh.PrivateKey
	if privateKey == "" && ssh.PrivateKeyFromSecret != nil {
		// Read private key from secret
		var err error
		secretNamespace := ssh.PrivateKeyFromSecret.Namespace
		if secretNamespace == "" {
			secretNamespace = namespace // fallback to resource namespace
		}
		privateKey, err = s.getPrivateKeyFromSecret(ctx, secretNamespace, ssh.PrivateKeyFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key from secret: %w", err)
		}
		// If from secret, the secret data is the actual key content (not base64)
		// We need to base64 encode it for the API
		privateKey = base64.StdEncoding.EncodeToString([]byte(privateKey))
	}

	// Get password (could be direct or from secret)
	password := ssh.Password
	if password == "" && ssh.PasswordFromSecret != nil {
		var err error
		secretNamespace := ssh.PasswordFromSecret.Namespace
		if secretNamespace == "" {
			secretNamespace = namespace // fallback to resource namespace
		}
		password, err = s.getSecretValue(ctx, secretNamespace, ssh.PasswordFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read password from secret: %w", err)
		}
	}

	// Ensure we have either private key or password for authentication
	if privateKey == "" && password == "" {
		return nil, fmt.Errorf("either private key or password must be provided for SSH source")
	}

	// Set default port if not specified
	port := ssh.Port
	if port == 0 {
		port = 22
	}

	// Build sync request with SSH source
	return &SyncRequest{
		Source: SyncSource{
			Type: "ssh",
			Details: SyncSourceSSH{
				User:       ssh.User,
				Host:       ssh.Host,
				Port:       port,
				Path:       ssh.Path,
				Password:   password,
				PrivateKey: privateKey,
			},
		},
		Target: SyncTarget{
			Path: targetPath,
		},
		Timeout: timeout,
	}, nil
}

// buildHTTPSyncRequestCommon builds sync request for HTTP source from VolumeSpecBase
func (s *SyncController) buildHTTPSyncRequestCommon(spec svv1alpha1.VolumeSpecBase, timeout, targetPath string) (*SyncRequest, error) {
	http := spec.Source.HTTP

	return &SyncRequest{
		Source: SyncSource{
			Type: "http",
			Details: SyncSourceHTTP{
				URL: http.URL,
			},
		},
		Target: SyncTarget{
			Path: targetPath,
		},
		Timeout: timeout,
	}, nil
}

// buildGitSyncRequestCommon builds sync request for Git source from VolumeSpecBase
func (s *SyncController) buildGitSyncRequestCommon(ctx context.Context, name, namespace string, spec svv1alpha1.VolumeSpecBase, timeout, targetPath string) (*SyncRequest, error) {
	git := spec.Source.Git

	// Get password (could be direct or from secret)
	password := git.Password
	if password == "" && git.PasswordFromSecret != nil {
		var err error
		secretNamespace := git.PasswordFromSecret.Namespace
		if secretNamespace == "" {
			secretNamespace = namespace // fallback to resource namespace
		}
		password, err = s.getSecretValue(ctx, secretNamespace, git.PasswordFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read password from secret: %w", err)
		}
	}

	// Get private key (could be direct or from secret)
	privateKey := git.PrivateKey
	if privateKey == "" && git.PrivateKeyFromSecret != nil {
		var err error
		secretNamespace := git.PrivateKeyFromSecret.Namespace
		if secretNamespace == "" {
			secretNamespace = namespace // fallback to resource namespace
		}
		privateKey, err = s.getPrivateKeyFromSecret(ctx, secretNamespace, git.PrivateKeyFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read private key from secret: %w", err)
		}
		// If from secret, the secret data is the actual key content (not base64)
		// We need to base64 encode it for the API
		privateKey = base64.StdEncoding.EncodeToString([]byte(privateKey))
	}

	return &SyncRequest{
		Source: SyncSource{
			Type: "git",
			Details: SyncSourceGit{
				URL:        git.URL,
				User:       git.User,
				Password:   password,
				PrivateKey: privateKey,
				Branch:     git.Branch,
			},
		},
		Target: SyncTarget{
			Path: targetPath,
		},
		Timeout: timeout,
	}, nil
}

// buildS3SyncRequestCommon builds sync request for S3 source from VolumeSpecBase
func (s *SyncController) buildS3SyncRequestCommon(ctx context.Context, name, namespace string, spec svv1alpha1.VolumeSpecBase, timeout, targetPath string) (*SyncRequest, error) {
	s3 := spec.Source.S3

	// Get access key (could be direct or from secret)
	accessKey := s3.AccessKey
	if accessKey == "" && s3.AccessKeyFromSecret != nil {
		var err error
		secretNamespace := s3.AccessKeyFromSecret.Namespace
		if secretNamespace == "" {
			secretNamespace = namespace // fallback to resource namespace
		}
		accessKey, err = s.getSecretValue(ctx, secretNamespace, s3.AccessKeyFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read access key from secret: %w", err)
		}
	}

	// Get secret key (could be direct or from secret)
	secretKey := s3.SecretKey
	if secretKey == "" && s3.SecretKeyFromSecret != nil {
		var err error
		secretNamespace := s3.SecretKeyFromSecret.Namespace
		if secretNamespace == "" {
			secretNamespace = namespace // fallback to resource namespace
		}
		secretKey, err = s.getSecretValue(ctx, secretNamespace, s3.SecretKeyFromSecret)
		if err != nil {
			return nil, fmt.Errorf("failed to read secret key from secret: %w", err)
		}
	}

	return &SyncRequest{
		Source: SyncSource{
			Type: "s3",
			Details: SyncSourceS3{
				BucketName:  s3.BucketName,
				Path:        s3.Path,
				Region:      s3.Region,
				AccessKey:   accessKey,
				SecretKey:   secretKey,
				EndpointURL: s3.EndpointURL,
			},
		},
		Target: SyncTarget{
			Path: targetPath,
		},
		Timeout: timeout,
	}, nil
}
