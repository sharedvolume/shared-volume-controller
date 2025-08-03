package validation

import (
	"testing"

	"k8s.io/apimachinery/pkg/util/validation/field"

	svv1alpha1 "github.com/sharedvolume/shared-volume-controller/api/v1alpha1"
)

const (
	testNfsServerImage  = "nfs-server:latest"
	testExternalNfsURL  = "192.168.1.100"
	testExternalNfsPath = "/shared/data"
	testForbiddenValue  = "should-not-be-here"
	testUserSpecified   = "user-specified"
)

func TestValidateNfsServerConfigForUpdate(t *testing.T) {
	tests := []struct {
		name         string
		oldNfsServer *svv1alpha1.NfsServerSpec
		newNfsServer *svv1alpha1.NfsServerSpec
		wantErrs     int
		description  string
	}{
		{
			name:         "both nil should pass",
			oldNfsServer: nil,
			newNfsServer: nil,
			wantErrs:     0,
			description:  "both old and new are nil",
		},
		{
			name:         "new nil should pass",
			oldNfsServer: &svv1alpha1.NfsServerSpec{Name: "test", Namespace: "test"},
			newNfsServer: nil,
			wantErrs:     0,
			description:  "removing nfs server config",
		},
		{
			name:         "controller setting managed fields should pass",
			oldNfsServer: nil,
			newNfsServer: &svv1alpha1.NfsServerSpec{
				Name:      "nfs-server-generated",
				Namespace: "default",
				Image:     testNfsServerImage,
				Path:      "/",
			},
			wantErrs:    0,
			description: "controller generating managed NFS server fields",
		},
		{
			name: "controller updating managed fields should pass",
			oldNfsServer: &svv1alpha1.NfsServerSpec{
				Name:      "nfs-server-old",
				Namespace: "default",
				Image:     "nfs-server:old",
				Path:      "/",
			},
			newNfsServer: &svv1alpha1.NfsServerSpec{
				Name:      "nfs-server-new",
				Namespace: "default",
				Image:     "nfs-server:new",
				Path:      "/",
			},
			wantErrs:    0,
			description: "controller updating managed NFS server fields",
		},
		{
			name:         "external NFS with valid config should pass",
			oldNfsServer: nil,
			newNfsServer: &svv1alpha1.NfsServerSpec{
				URL:  testExternalNfsURL,
				Path: testExternalNfsPath,
			},
			wantErrs:    0,
			description: "user setting external NFS server config",
		},
		{
			name:         "external NFS with invalid config should pass (no validation)",
			oldNfsServer: nil,
			newNfsServer: &svv1alpha1.NfsServerSpec{
				URL: testExternalNfsURL,
				// Missing Path - but no validation anymore
			},
			wantErrs:    0,
			description: "user setting incomplete external NFS server config",
		},
		{
			name:         "external NFS with managed fields should pass (no validation)",
			oldNfsServer: nil,
			newNfsServer: &svv1alpha1.NfsServerSpec{
				URL:       testExternalNfsURL,
				Path:      testExternalNfsPath,
				Name:      testForbiddenValue, // No longer forbidden
				Namespace: testForbiddenValue, // No longer forbidden
			},
			wantErrs:    0,
			description: "user setting external NFS with managed fields",
		},
		{
			name:         "external NFS with image field should pass (no validation)",
			oldNfsServer: nil,
			newNfsServer: &svv1alpha1.NfsServerSpec{
				URL:   testExternalNfsURL,
				Path:  testExternalNfsPath,
				Image: testForbiddenValue, // No longer forbidden
			},
			wantErrs:    0,
			description: "user setting external NFS with image field",
		},
		{
			name: "updating from managed to external should pass",
			oldNfsServer: &svv1alpha1.NfsServerSpec{
				Name:      "nfs-server-managed",
				Namespace: "default",
				Image:     testNfsServerImage,
				Path:      "/",
			},
			newNfsServer: &svv1alpha1.NfsServerSpec{
				URL:  testExternalNfsURL,
				Path: testExternalNfsPath,
			},
			wantErrs:    0,
			description: "changing from managed to external NFS",
		},
		{
			name: "updating from external to managed should pass",
			oldNfsServer: &svv1alpha1.NfsServerSpec{
				URL:  testExternalNfsURL,
				Path: testExternalNfsPath,
			},
			newNfsServer: &svv1alpha1.NfsServerSpec{
				Name:      "nfs-server-generated",
				Namespace: "default",
				Image:     testNfsServerImage,
				Path:      "/",
			},
			wantErrs:    0,
			description: "controller changing from external to managed NFS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fldPath := field.NewPath("spec", "nfsServer")
			errs := validateNfsServerConfigForUpdate(tt.oldNfsServer, tt.newNfsServer, fldPath)

			if len(errs) != tt.wantErrs {
				t.Errorf("validateNfsServerConfigForUpdate() got %d errors, want %d for case: %s. Errors: %v",
					len(errs), tt.wantErrs, tt.description, errs)
			}
		})
	}
}

func TestValidateNfsServerConfigForCreate(t *testing.T) {
	tests := []struct {
		name        string
		nfsServer   *svv1alpha1.NfsServerSpec
		wantErrs    int
		description string
	}{
		{
			name:        "nil should pass",
			nfsServer:   nil,
			wantErrs:    0,
			description: "no nfs server config",
		},
		{
			name:        "empty should pass",
			nfsServer:   &svv1alpha1.NfsServerSpec{},
			wantErrs:    0,
			description: "empty nfs server config",
		},
		{
			name: "managed fields should pass (no validation)",
			nfsServer: &svv1alpha1.NfsServerSpec{
				Name:      testUserSpecified, // No longer forbidden
				Namespace: testUserSpecified, // No longer forbidden
			},
			wantErrs:    0,
			description: "user specifying managed fields on create",
		},
		{
			name: "external config should pass",
			nfsServer: &svv1alpha1.NfsServerSpec{
				URL:  testExternalNfsURL,
				Path: testExternalNfsPath,
			},
			wantErrs:    0,
			description: "valid external NFS config",
		},
		{
			name: "external config missing path should pass (no validation)",
			nfsServer: &svv1alpha1.NfsServerSpec{
				URL: testExternalNfsURL,
				// Missing Path - but no validation anymore
			},
			wantErrs:    0,
			description: "external NFS config missing path",
		},
		{
			name: "external config missing url should pass (no validation)",
			nfsServer: &svv1alpha1.NfsServerSpec{
				Path: testExternalNfsPath,
				// Missing URL - but no validation anymore
			},
			wantErrs:    0,
			description: "external NFS config missing url",
		},
		{
			name: "external config with image should pass (no validation)",
			nfsServer: &svv1alpha1.NfsServerSpec{
				URL:   testExternalNfsURL,
				Path:  testExternalNfsPath,
				Image: testForbiddenValue, // No longer forbidden
			},
			wantErrs:    0,
			description: "external NFS config with image field",
		},
		{
			name: "image field without external config should pass (no validation)",
			nfsServer: &svv1alpha1.NfsServerSpec{
				Image: testUserSpecified, // No longer forbidden
			},
			wantErrs:    0,
			description: "user specifying image field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fldPath := field.NewPath("spec", "nfsServer")
			errs := validateNfsServerConfigForCreate(tt.nfsServer, fldPath)

			if len(errs) != tt.wantErrs {
				t.Errorf("validateNfsServerConfigForCreate() got %d errors, want %d for case: %s. Errors: %v",
					len(errs), tt.wantErrs, tt.description, errs)
			}
		})
	}
}
