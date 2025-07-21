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

package validation

import (
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

// ValidateVolumeSpec validates the common volume specification
func ValidateVolumeSpec(spec *svv1alpha1.VolumeSpecBase, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	// Validate mandatory fields
	if err := validateMandatoryFields(spec, fldPath); err != nil {
		allErrs = append(allErrs, err...)
	}

	// Validate source configuration
	if err := validateSourceSpec(spec.Source, fldPath.Child("source"), isClusterScoped); err != nil {
		allErrs = append(allErrs, err...)
	}

	// Validate sync configuration
	if err := validateSyncConfig(spec, fldPath); err != nil {
		allErrs = append(allErrs, err...)
	}

	return allErrs
}

// validateMandatoryFields validates required fields
func validateMandatoryFields(spec *svv1alpha1.VolumeSpecBase, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// mountPath is mandatory
	if spec.MountPath == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("mountPath"), "mountPath is required"))
	}

	return allErrs
}

// validateSourceSpec validates source configuration
func validateSourceSpec(source *svv1alpha1.VolumeSourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	if source == nil {
		return allErrs
	}

	// Validate source count and individual sources
	if err := validateSourceCount(source, fldPath); err != nil {
		allErrs = append(allErrs, err)
		return allErrs
	}

	// Validate individual source types
	allErrs = append(allErrs, validateIndividualSources(source, fldPath, isClusterScoped)...)

	return allErrs
}

// validateSourceCount ensures only one source type is specified
func validateSourceCount(source *svv1alpha1.VolumeSourceSpec, fldPath *field.Path) *field.Error {
	sourceCount := 0
	if source.SSH != nil {
		sourceCount++
	}
	if source.HTTP != nil {
		sourceCount++
	}
	if source.Git != nil {
		sourceCount++
	}
	if source.S3 != nil {
		sourceCount++
	}

	if sourceCount > 1 {
		return field.Invalid(fldPath, source, "source must have at most one source type (ssh, http, git, or s3)")
	}
	return nil
}

// validateIndividualSources validates each source type
func validateIndividualSources(source *svv1alpha1.VolumeSourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	if source.SSH != nil {
		allErrs = append(allErrs, validateSSHSource(source.SSH, fldPath.Child("ssh"), isClusterScoped)...)
	}

	if source.HTTP != nil {
		allErrs = append(allErrs, validateHTTPSource(source.HTTP, fldPath.Child("http"))...)
	}

	if source.Git != nil {
		allErrs = append(allErrs, validateGitSource(source.Git, fldPath.Child("git"), isClusterScoped)...)
	}

	if source.S3 != nil {
		allErrs = append(allErrs, validateS3Source(source.S3, fldPath.Child("s3"), isClusterScoped)...)
	}

	return allErrs
}

// validateSSHSource validates SSH source configuration
func validateSSHSource(ssh *svv1alpha1.SSHSourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	// host is mandatory
	if ssh.Host == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("host"), "host is required for SSH source"))
	}

	// path is mandatory
	if ssh.Path == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("path"), "path is required for SSH source"))
	}

	// Validate secret references
	if err := validateSecretReferences(ssh, fldPath, isClusterScoped); err != nil {
		allErrs = append(allErrs, err...)
	}

	return allErrs
}

// validateSecretReferences validates SSH secret reference configurations
func validateSecretReferences(ssh *svv1alpha1.SSHSourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	// Validate privateKey references
	allErrs = append(allErrs, validatePrivateKeyReferences(ssh, fldPath, isClusterScoped)...)

	// Validate password references
	allErrs = append(allErrs, validatePasswordReferences(ssh, fldPath, isClusterScoped)...)

	return allErrs
}

// validatePrivateKeyReferences validates privateKey reference configurations
func validatePrivateKeyReferences(ssh *svv1alpha1.SSHSourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	privateKeyCount := 0
	if ssh.PrivateKey != "" {
		privateKeyCount++
	}
	if ssh.PrivateKeyFromSecret != nil {
		privateKeyCount++
		if err := validateSecretKeySelector(ssh.PrivateKeyFromSecret, fldPath.Child("privateKeyFromSecret"), isClusterScoped); err != nil {
			allErrs = append(allErrs, err...)
		}
	}

	if privateKeyCount > 1 {
		allErrs = append(allErrs, field.Invalid(fldPath, ssh, "only one of privateKey or privateKeyFromSecret should be specified"))
	}

	return allErrs
}

// validatePasswordReferences validates password reference configurations
func validatePasswordReferences(ssh *svv1alpha1.SSHSourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	passwordCount := 0
	if ssh.Password != "" {
		passwordCount++
	}
	if ssh.PasswordFromSecret != nil {
		passwordCount++
		if err := validateSecretKeySelector(ssh.PasswordFromSecret, fldPath.Child("passwordFromSecret"), isClusterScoped); err != nil {
			allErrs = append(allErrs, err...)
		}
	}

	if passwordCount > 1 {
		allErrs = append(allErrs, field.Invalid(fldPath, ssh, "only one of password or passwordFromSecret should be specified"))
	}

	return allErrs
}

// validateHTTPSource validates HTTP source configuration
func validateHTTPSource(http *svv1alpha1.HTTPSourceSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// url is mandatory
	if http.URL == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("url"), "url is required for HTTP source"))
	}

	return allErrs
}

// validateGitSource validates Git source configuration
func validateGitSource(git *svv1alpha1.GitSourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	// url is mandatory
	if git.URL == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("url"), "url is required for Git source"))
	}

	// Validate secret references for git
	if err := validateGitSecretReferences(git, fldPath, isClusterScoped); err != nil {
		allErrs = append(allErrs, err...)
	}

	return allErrs
}

// validateGitSecretReferences validates Git secret reference configurations
func validateGitSecretReferences(git *svv1alpha1.GitSourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	// Validate privateKey references
	allErrs = append(allErrs, validateGitPrivateKeyReferences(git, fldPath, isClusterScoped)...)

	// Validate password references
	allErrs = append(allErrs, validateGitPasswordReferences(git, fldPath, isClusterScoped)...)

	return allErrs
}

// validateGitPrivateKeyReferences validates Git privateKey reference configurations
func validateGitPrivateKeyReferences(git *svv1alpha1.GitSourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	privateKeyCount := 0
	if git.PrivateKey != "" {
		privateKeyCount++
	}
	if git.PrivateKeyFromSecret != nil {
		privateKeyCount++
		if err := validateSecretKeySelector(git.PrivateKeyFromSecret, fldPath.Child("privateKeyFromSecret"), isClusterScoped); err != nil {
			allErrs = append(allErrs, err...)
		}
	}

	if privateKeyCount > 1 {
		allErrs = append(allErrs, field.Invalid(fldPath, git, "only one of privateKey or privateKeyFromSecret should be specified"))
	}

	return allErrs
}

// validateGitPasswordReferences validates Git password reference configurations
func validateGitPasswordReferences(git *svv1alpha1.GitSourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	passwordCount := 0
	if git.Password != "" {
		passwordCount++
	}
	if git.PasswordFromSecret != nil {
		passwordCount++
		if err := validateSecretKeySelector(git.PasswordFromSecret, fldPath.Child("passwordFromSecret"), isClusterScoped); err != nil {
			allErrs = append(allErrs, err...)
		}
	}

	if passwordCount > 1 {
		allErrs = append(allErrs, field.Invalid(fldPath, git, "only one of password or passwordFromSecret should be specified"))
	}

	return allErrs
}

// validateS3Source validates S3 source configuration
func validateS3Source(s3 *svv1alpha1.S3SourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	// endpointUrl is mandatory
	if s3.EndpointURL == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("endpointUrl"), "endpointUrl is required for S3 source"))
	}

	// bucketName is mandatory
	if s3.BucketName == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("bucketName"), "bucketName is required for S3 source"))
	}

	// region is mandatory
	if s3.Region == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("region"), "region is required for S3 source"))
	}

	// Validate access key - either direct value or from secret
	if err := validateS3AccessKey(s3, fldPath, isClusterScoped); err != nil {
		allErrs = append(allErrs, err...)
	}

	// Validate secret key - either direct value or from secret
	if err := validateS3SecretKey(s3, fldPath, isClusterScoped); err != nil {
		allErrs = append(allErrs, err...)
	}

	return allErrs
}

// validateS3AccessKey validates S3 access key configurations
func validateS3AccessKey(s3 *svv1alpha1.S3SourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	accessKeyCount := 0
	if s3.AccessKey != "" {
		accessKeyCount++
	}
	if s3.AccessKeyFromSecret != nil {
		accessKeyCount++
		if err := validateSecretKeySelector(s3.AccessKeyFromSecret, fldPath.Child("accessKeyFromSecret"), isClusterScoped); err != nil {
			allErrs = append(allErrs, err...)
		}
	}

	if accessKeyCount == 0 {
		allErrs = append(allErrs, field.Required(fldPath, "one of accessKey or accessKeyFromSecret is required"))
	} else if accessKeyCount > 1 {
		allErrs = append(allErrs, field.Invalid(fldPath, s3, "only one of accessKey or accessKeyFromSecret should be specified"))
	}

	return allErrs
}

// validateS3SecretKey validates S3 secret key configurations
func validateS3SecretKey(s3 *svv1alpha1.S3SourceSpec, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	secretKeyCount := 0
	if s3.SecretKey != "" {
		secretKeyCount++
	}
	if s3.SecretKeyFromSecret != nil {
		secretKeyCount++
		if err := validateSecretKeySelector(s3.SecretKeyFromSecret, fldPath.Child("secretKeyFromSecret"), isClusterScoped); err != nil {
			allErrs = append(allErrs, err...)
		}
	}

	if secretKeyCount == 0 {
		allErrs = append(allErrs, field.Required(fldPath, "one of secretKey or secretKeyFromSecret is required"))
	} else if secretKeyCount > 1 {
		allErrs = append(allErrs, field.Invalid(fldPath, s3, "only one of secretKey or secretKeyFromSecret should be specified"))
	}

	return allErrs
}

// validateSyncConfig validates sync configuration
func validateSyncConfig(spec *svv1alpha1.VolumeSpecBase, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if spec.SyncInterval == "" || spec.SyncTimeout == "" {
		return allErrs
	}

	// Parse sync interval
	syncInterval, err := time.ParseDuration(spec.SyncInterval)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("syncInterval"), spec.SyncInterval, "invalid duration format"))
		return allErrs
	}

	// Parse sync timeout
	syncTimeout, err := time.ParseDuration(spec.SyncTimeout)
	if err != nil {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("syncTimeout"), spec.SyncTimeout, "invalid duration format"))
		return allErrs
	}

	// syncTimeout must be smaller than syncInterval
	if syncTimeout >= syncInterval {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("syncTimeout"), spec.SyncTimeout, "syncTimeout must be smaller than syncInterval"))
	}

	return allErrs
}

// ValidateVolumeObject validates a volume object (SharedVolume or ClusterSharedVolume)
func ValidateVolumeObject(obj metav1.Object, spec *svv1alpha1.VolumeSpecBase, kind string) error {
	var allErrs field.ErrorList

	// Determine if this is a cluster-scoped resource
	isClusterScoped := kind == "ClusterSharedVolume"

	// Validate the spec
	if specErrs := ValidateVolumeSpec(spec, field.NewPath("spec"), isClusterScoped); len(specErrs) > 0 {
		allErrs = append(allErrs, specErrs...)
	}

	if len(allErrs) == 0 {
		return nil
	}

	return fmt.Errorf("%s validation failed: %v", kind, allErrs)
}

// validateSecretKeySelector validates a SecretKeySelector
func validateSecretKeySelector(selector *svv1alpha1.SecretKeySelector, fldPath *field.Path, isClusterScoped bool) field.ErrorList {
	var allErrs field.ErrorList

	if selector.Name == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("name"), "name is required"))
	}

	if selector.Key == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("key"), "key is required"))
	}

	// For cluster-scoped resources, namespace is required
	if isClusterScoped && selector.Namespace == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("namespace"), "namespace is required for cluster-scoped resources"))
	}

	return allErrs
}
