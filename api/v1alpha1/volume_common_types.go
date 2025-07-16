/*
Common types for SharedVolume specs.
*/

package v1alpha1

// NfsServerSpec defines the NFS server configuration.
type NfsServerSpec struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	URL       string `json:"url,omitempty"`
	Image     string `json:"image,omitempty"` // Docker image for the NFS server
	Path      string `json:"path,omitempty"`  // Path on the NFS server to mount
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
	User                 string             `json:"user,omitempty"`
	Password             string             `json:"password,omitempty"`
	PrivateKey           string             `json:"privateKey,omitempty"`
	Path                 string             `json:"path,omitempty"`
	PrivateKeyFromSecret *SecretKeySelector `json:"privateKeyFromSecret,omitempty"`
}

// HTTPSourceSpec defines HTTP source configuration.
type HTTPSourceSpec struct {
	URL string `json:"url"`
}

// GitSourceSpec defines Git source configuration.
type GitSourceSpec struct {
	URL        string `json:"url"`
	User       string `json:"user,omitempty"`
	Password   string `json:"password,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
	Branch     string `json:"branch,omitempty"`
}

// S3SourceSpec defines S3 source configuration.
type S3SourceSpec struct {
	EndpointURL string `json:"endpointUrl"`
	BucketName  string `json:"bucketName"`
	Path        string `json:"path,omitempty"`
	AccessKey   string `json:"accessKey"`
	SecretKey   string `json:"secretKey"`
	Region      string `json:"region"`
}

type SecretKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// VolumeSourceSpec defines the source for the shared volume.
type VolumeSourceSpec struct {
	SSH  *SSHSourceSpec  `json:"ssh,omitempty"`
	HTTP *HTTPSourceSpec `json:"http,omitempty"`
	Git  *GitSourceSpec  `json:"git,omitempty"`
	S3   *S3SourceSpec   `json:"s3,omitempty"`
}

// VolumeSpecBase contains common fields for SharedVolume.
type VolumeSpecBase struct {
	NfsServer         *NfsServerSpec    `json:"nfsServer,omitempty"`
	MountPath         string            `json:"mountPath"`
	SyncInterval      string            `json:"syncInterval,omitempty"` // Sync interval like 30s, 5m, 2h, 1d
	SyncTimeout       string            `json:"syncTimeout,omitempty"`  // Timeout for sync operations like 120s
	Storage           *StorageSpec      `json:"storage,omitempty"`
	Source            *VolumeSourceSpec `json:"source,omitempty"`
	StorageClassName  string            `json:"storageClassName,omitempty"`
	ResourceNamespace string            `json:"resourceNamespace,omitempty"` // Namespace for the resource
	ReferenceValue    string            `json:"referenceValue,omitempty"`    // Shared reference value for related resources
}
