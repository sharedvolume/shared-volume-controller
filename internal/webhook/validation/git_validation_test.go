package validation

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/validation/field"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

func TestValidateGitPasswordReferences(t *testing.T) {
	tests := []struct {
		name     string
		git      *svv1alpha1.GitSourceSpec
		wantErrs int
		errField string
	}{
		{
			name: "password without user should fail",
			git: &svv1alpha1.GitSourceSpec{
				URL:      "https://github.com/example/repo.git",
				Password: "secret123",
				// User is missing
			},
			wantErrs: 1,
			errField: "user",
		},
		{
			name: "passwordFromSecret without user should fail",
			git: &svv1alpha1.GitSourceSpec{
				URL: "https://github.com/example/repo.git",
				PasswordFromSecret: &svv1alpha1.SecretKeySelector{
					Name: "git-secret",
					Key:  "password",
				},
				// User is missing
			},
			wantErrs: 1,
			errField: "user",
		},
		{
			name: "password with user should pass",
			git: &svv1alpha1.GitSourceSpec{
				URL:      "https://github.com/example/repo.git",
				User:     "testuser",
				Password: "secret123",
			},
			wantErrs: 0,
		},
		{
			name: "passwordFromSecret with user should pass",
			git: &svv1alpha1.GitSourceSpec{
				URL:  "https://github.com/example/repo.git",
				User: "testuser",
				PasswordFromSecret: &svv1alpha1.SecretKeySelector{
					Name: "git-secret",
					Key:  "password",
				},
			},
			wantErrs: 0,
		},
		{
			name: "no password should pass",
			git: &svv1alpha1.GitSourceSpec{
				URL: "https://github.com/example/repo.git",
				// No password, no user - should be valid
			},
			wantErrs: 0,
		},
		{
			name: "user without password should pass",
			git: &svv1alpha1.GitSourceSpec{
				URL:  "https://github.com/example/repo.git",
				User: "testuser",
				// No password - should be valid
			},
			wantErrs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fldPath := field.NewPath("spec", "source", "git")
			errs := validateGitPasswordReferences(tt.git, fldPath, false)

			if len(errs) != tt.wantErrs {
				t.Errorf("validateGitPasswordReferences() got %d errors, want %d. Errors: %v", len(errs), tt.wantErrs, errs)
				return
			}

			if tt.wantErrs > 0 && tt.errField != "" {
				found := false
				for _, err := range errs {
					if err.Type == field.ErrorTypeRequired && err.Field == fldPath.Child(tt.errField).String() {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected required error for field %s, but not found in errors: %v", tt.errField, errs)
				}
			}
		})
	}
}
