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

package utils

import (
	"fmt"
	"strings"

	"github.com/sharedvolume/shared-volume-controller/internal/controller/types"
)

// NameGenerator provides utilities for generating resource names
type NameGenerator struct {
	RandomGen *RandomGenerator
}

// NewNameGenerator creates a new NameGenerator instance
func NewNameGenerator() *NameGenerator {
	return &NameGenerator{
		RandomGen: NewRandomGenerator(),
	}
}

// GeneratePVCName generates a PVC name using the standard pattern
func (n *NameGenerator) GeneratePVCName(referenceValue, namespace string) string {
	return fmt.Sprintf(types.PVCNameTemplate, referenceValue, namespace)
}

// GeneratePVName generates a PV name using the standard pattern
func (n *NameGenerator) GeneratePVName(referenceValue, namespace string) string {
	return fmt.Sprintf(types.PVNameTemplate, referenceValue, namespace)
}

// GenerateServiceName generates a service name using the standard pattern
func (n *NameGenerator) GenerateServiceName(referenceValue string) string {
	return fmt.Sprintf(types.ServiceNameTemplate, referenceValue)
}

// GenerateReplicaSetName generates a ReplicaSet name using the standard pattern
func (n *NameGenerator) GenerateReplicaSetName(referenceValue string) string {
	return fmt.Sprintf(types.ReplicaSetNameTemplate, referenceValue)
}

// GenerateNfsServerName generates an NFS server name
func (n *NameGenerator) GenerateNfsServerName(volumeName string) string {
	// Generate a shorter random suffix for NFS server names
	randomSuffix := n.RandomGen.RandString(6)
	return fmt.Sprintf("%s-nfs-%s", volumeName, randomSuffix)
}

// GenerateResourcePrefix generates a resource prefix with random suffix
func (n *NameGenerator) GenerateResourcePrefix(baseName string, randomLength int) string {
	randomSuffix := n.RandomGen.RandString(randomLength)
	return fmt.Sprintf("%s-%s", baseName, randomSuffix)
}

// SanitizeName sanitizes a name to be valid for Kubernetes resources
func (n *NameGenerator) SanitizeName(name string) string {
	// Convert to lowercase
	sanitized := strings.ToLower(name)

	// Replace invalid characters with hyphens
	sanitized = strings.ReplaceAll(sanitized, "_", "-")
	sanitized = strings.ReplaceAll(sanitized, ".", "-")
	sanitized = strings.ReplaceAll(sanitized, " ", "-")

	// Remove consecutive hyphens
	for strings.Contains(sanitized, "--") {
		sanitized = strings.ReplaceAll(sanitized, "--", "-")
	}

	// Remove leading/trailing hyphens
	sanitized = strings.Trim(sanitized, "-")

	// Ensure name is not empty
	if sanitized == "" {
		sanitized = "unnamed"
	}

	// Ensure name doesn't exceed Kubernetes limits (63 characters for most resources)
	if len(sanitized) > 63 {
		sanitized = sanitized[:63]
		sanitized = strings.TrimSuffix(sanitized, "-")
	}

	return sanitized
}

// TruncateWithSuffix truncates a name to fit within length limits while preserving a suffix
func (n *NameGenerator) TruncateWithSuffix(name, suffix string, maxLength int) string {
	if len(name)+len(suffix) <= maxLength {
		return name + suffix
	}

	// Calculate how much of the name we can keep
	nameLength := maxLength - len(suffix)
	if nameLength <= 0 {
		// Suffix is too long, just return truncated suffix
		if len(suffix) > maxLength {
			return suffix[:maxLength]
		}
		return suffix
	}

	truncatedName := name[:nameLength]
	truncatedName = strings.TrimSuffix(truncatedName, "-")

	return truncatedName + suffix
}

// GenerateUniqueResourceName generates a unique resource name with validation
func (n *NameGenerator) GenerateUniqueResourceName(baseName, resourceType string, maxLength int) string {
	// Sanitize the base name
	sanitizedBase := n.SanitizeName(baseName)

	// Add resource type suffix
	suffix := fmt.Sprintf("-%s", resourceType)

	// Add random suffix for uniqueness
	randomSuffix := fmt.Sprintf("-%s", n.RandomGen.RandString(6))
	fullSuffix := suffix + randomSuffix

	// Truncate if necessary
	finalName := n.TruncateWithSuffix(sanitizedBase, fullSuffix, maxLength)

	return finalName
}
