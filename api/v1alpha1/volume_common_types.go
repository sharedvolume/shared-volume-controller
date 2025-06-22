/*
Common types for SharedVolume and ClusterSharedVolume specs.
*/

package v1alpha1

import "fmt"

// NfsServerSpec defines the NFS server configuration.
type NfsServerSpec struct {
	Type      string `json:"type"`
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	URL       string `json:"url,omitempty"`
}

// StorageSpec defines storage configuration.
type StorageSpec struct {
	Capacity   string `json:"capacity"`
	AccessMode string `json:"accessMode,omitempty"` // ReadWrite or ReadOnly
}

// SSHSourceSpec defines SSH source configuration.
type SSHSourceSpec struct {
	Host                 string             `json:"host"`
	Port                 int                `json:"port,omitempty"`
	Username             string             `json:"username,omitempty"`
	PrivateKey           string             `json:"privateKey,omitempty"`
	Path                 string             `json:"path,omitempty"`
	PrivateKeyFromSecret *SecretKeySelector `json:"privateKeyFromSecret,omitempty"`
}

type SecretKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// VolumeSourceSpec defines the source for the shared volume.
type VolumeSourceSpec struct {
	SSH *SSHSourceSpec `json:"ssh,omitempty"`
}

// VolumeSpecBase contains common fields for SharedVolume and ClusterSharedVolume.
type VolumeSpecBase struct {
	NfsServer        *NfsServerSpec    `json:"nfsServer,omitempty"`
	MountPath        string            `json:"mountPath"`
	SyncInterval     string            `json:"syncInterval,omitempty"`
	StorageClassName string            `json:"storageClassName,omitempty"`
	Storage          *StorageSpec      `json:"storage,omitempty"`
	Source           *VolumeSourceSpec `json:"source,omitempty"`
}

// SetDefaults sets default values for VolumeSpecBase fields.
func (v *VolumeSpecBase) SetDefaults(namespace string) {
	if v.SyncInterval == "" {
		v.SyncInterval = "60s"
	}
	if v.Storage != nil && v.Storage.AccessMode == "" {
		v.Storage.AccessMode = "ReadOnly"
	}
	if v.NfsServer != nil && v.NfsServer.Type == "object" && v.NfsServer.Namespace == "" {
		v.NfsServer.Namespace = namespace
	}
}

// Validate checks required fields and returns an error if invalid.
func (v *VolumeSpecBase) Validate() error {
	if v.MountPath == "" {
		return fmt.Errorf("mountPath is required")
	}
	if v.SyncInterval == "" {
		return fmt.Errorf("syncInterval is required")
	}
	if v.Storage == nil || v.Storage.Capacity == "" {
		return fmt.Errorf("storage.capacity is required")
	}
	if v.NfsServer == nil && v.StorageClassName == "" {
		return fmt.Errorf("storageClassName is required if nfsServer is not defined")
	}
	if v.NfsServer != nil {
		switch v.NfsServer.Type {
		case "object":
			if v.NfsServer.Name == "" {
				return fmt.Errorf("nfsServer.name is required when type is 'object'")
			}
		case "external":
			if v.NfsServer.URL == "" {
				return fmt.Errorf("nfsServer.url is required when type is 'external'")
			}
		}
	}
	return nil
}
