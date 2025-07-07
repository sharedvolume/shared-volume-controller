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
)

// GeneratePVCName generates a PVC name based on reference value and namespace
func GeneratePVCName(referenceValue, namespace string) string {
	return fmt.Sprintf("%s-%s", referenceValue, namespace)
}

// GeneratePVName generates a PV name based on reference value and namespace
func GeneratePVName(referenceValue, namespace string) string {
	return fmt.Sprintf("%s-%s", referenceValue, namespace)
}

// GenerateVolumeNameForPod generates a volume name for use in pod spec
func GenerateVolumeNameForPod(referenceValue string) string {
	return referenceValue
}

// SanitizeName sanitizes a name to be Kubernetes-compliant
func SanitizeName(name string) string {
	// Replace any invalid characters with hyphens
	// This is a simple implementation - you might want to make it more sophisticated
	sanitized := ""
	for _, char := range name {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			sanitized += string(char)
		} else {
			sanitized += "-"
		}
	}
	return sanitized
}
