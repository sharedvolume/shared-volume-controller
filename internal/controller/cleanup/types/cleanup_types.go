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

package types

// VolumeRef represents a reference to either a SharedVolume or ClusterSharedVolume
type VolumeRef struct {
	Name      string
	Namespace string
	IsCluster bool // true for ClusterSharedVolume, false for SharedVolume
}

// SharedVolumeRef represents a reference to a SharedVolume (legacy, for backward compatibility)
type SharedVolumeRef struct {
	Namespace string
	Name      string
}

// CleanupConfig holds configuration for cleanup operations
type CleanupConfig struct {
	MaxRetries                int
	ForceDeleteTimeoutSeconds int
	CleanupBatchSize          int
}
