# API Reference

This document provides detailed information about the Shared Volume Controller API resources.

## SharedVolume

SharedVolume is a namespace-scoped resource that creates shared storage within a specific namespace.

### Specification

```yaml
apiVersion: sv.sharedvolume.io/v1alpha1
kind: SharedVolume
metadata:
  name: example-shared-volume
  namespace: default
spec:
  capacity: "10Gi"
  accessModes:
    - ReadWriteMany
  storageClassName: "standard"
  source:
    type: "git"
    git:
      repository: "https://github.com/example/repo.git"
      branch: "main"
      path: "/data"
  replicas: 1
  syncPolicy:
    interval: "5m"
    retryLimit: 3
```

### Fields

#### SharedVolumeSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `capacity` | string | Yes | Storage capacity (e.g., "10Gi", "1Ti") |
| `accessModes` | []string | Yes | Volume access modes. Supported: "ReadWriteOnce", "ReadOnlyMany", "ReadWriteMany" |
| `storageClassName` | string | No | Storage class name. If empty, uses default storage class |
| `source` | SourceSpec | No | Data source configuration for volume content |
| `replicas` | int | No | Number of NFS server replicas (default: 1) |
| `syncPolicy` | SyncPolicySpec | No | Synchronization policy for source data |

#### SourceSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | Yes | Source type: "git", "ssh", "s3", or "nfs" |
| `git` | GitSourceSpec | No | Git source configuration |
| `ssh` | SSHSourceSpec | No | SSH source configuration |
| `s3` | S3SourceSpec | No | S3 source configuration |
| `nfs` | NFSSourceSpec | No | NFS source configuration |

#### GitSourceSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `repository` | string | Yes | Git repository URL |
| `branch` | string | No | Branch name (default: "main") |
| `tag` | string | No | Git tag (mutually exclusive with branch) |
| `path` | string | No | Subdirectory within repository |
| `credentials` | CredentialsSpec | No | Authentication credentials |

#### CredentialsSpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `secretRef` | SecretReference | No | Reference to secret containing credentials |
| `username` | string | No | Username for authentication |
| `password` | string | No | Password for authentication |
| `privateKey` | string | No | Private key for SSH authentication |

#### SyncPolicySpec

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `interval` | string | No | Sync interval (e.g., "5m", "1h") |
| `retryLimit` | int | No | Maximum retry attempts |
| `timeout` | string | No | Sync operation timeout |

### Status

#### SharedVolumeStatus

| Field | Type | Description |
|-------|------|-------------|
| `phase` | string | Current phase: "Pending", "Preparing", "Ready", "Failed" |
| `message` | string | Human-readable status message |
| `nfsServerAddress` | string | NFS server endpoint address |
| `persistentVolumeClaimName` | string | Associated PVC name |
| `persistentVolumeName` | string | Associated PV name |
| `serviceName` | string | NFS service name |
| `lastSyncTime` | metav1.Time | Last successful sync timestamp |
| `syncStatus` | string | Sync operation status |

## ClusterSharedVolume

ClusterSharedVolume is a cluster-scoped resource that creates shared storage accessible across namespaces.

### Specification

```yaml
apiVersion: sv.sharedvolume.io/v1alpha1
kind: ClusterSharedVolume
metadata:
  name: cluster-config-volume
spec:
  capacity: "5Gi"
  accessModes:
    - ReadOnlyMany
  storageClassName: "fast-ssd"
  namespaceSelector:
    matchLabels:
      shared-volume: "enabled"
  source:
    type: "git"
    git:
      repository: "https://github.com/company/config.git"
      branch: "production"
```

### Fields

#### ClusterSharedVolumeSpec

Inherits all fields from SharedVolumeSpec with additional fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `namespaceSelector` | metav1.LabelSelector | No | Selector for allowed namespaces |
| `allowedNamespaces` | []string | No | Explicit list of allowed namespaces |

## Pod Annotations

The controller supports automatic volume mounting through pod annotations:

### SharedVolume Mounting

```yaml
annotations:
  sv.sharedvolume.io/mount: "volume-name:/mount/path"
  sv.sharedvolume.io/mount-multiple: "vol1:/path1,vol2:/path2"
```

### ClusterSharedVolume Mounting

```yaml
annotations:
  csv.sharedvolume.io/mount: "cluster-volume-name:/mount/path"
  csv.sharedvolume.io/mount-multiple: "cvol1:/path1,cvol2:/path2"
```

### Annotation Options

| Annotation | Description | Example |
|------------|-------------|---------|
| `sv.sharedvolume.io/mount` | Mount single SharedVolume | `"data-volume:/app/data"` |
| `sv.sharedvolume.io/mount-multiple` | Mount multiple SharedVolumes | `"vol1:/data,vol2:/logs"` |
| `csv.sharedvolume.io/mount` | Mount single ClusterSharedVolume | `"config:/app/config"` |
| `csv.sharedvolume.io/mount-multiple` | Mount multiple ClusterSharedVolumes | `"config:/app/config,certs:/etc/ssl"` |
| `sv.sharedvolume.io/readonly` | Mount as read-only | `"true"` |

## Examples

### Basic SharedVolume

```yaml
apiVersion: sv.sharedvolume.io/v1alpha1
kind: SharedVolume
metadata:
  name: app-data
  namespace: my-app
spec:
  capacity: "20Gi"
  accessModes:
    - ReadWriteMany
  storageClassName: "standard"
```

### Git-synchronized ClusterSharedVolume

```yaml
apiVersion: sv.sharedvolume.io/v1alpha1
kind: ClusterSharedVolume
metadata:
  name: shared-config
spec:
  capacity: "1Gi"
  accessModes:
    - ReadOnlyMany
  source:
    type: "git"
    git:
      repository: "https://github.com/company/config.git"
      branch: "main"
      credentials:
        secretRef:
          name: git-credentials
          namespace: shared-volume-controller-system
  syncPolicy:
    interval: "10m"
    retryLimit: 5
  namespaceSelector:
    matchLabels:
      environment: "production"
```

### Pod with Automatic Volume Mounting

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: app-pod
  namespace: my-app
  annotations:
    sv.sharedvolume.io/mount: "app-data:/app/data"
    csv.sharedvolume.io/mount: "shared-config:/app/config"
    sv.sharedvolume.io/readonly: "false"
spec:
  containers:
  - name: app
    image: my-app:latest
    # Volumes will be automatically injected by the webhook
```

## Webhook Configuration

The controller includes admission webhooks for validation and mutation:

### Validating Webhooks

- **SharedVolume**: Validates resource specifications
- **ClusterSharedVolume**: Validates cluster-wide resource specifications

### Mutating Webhooks

- **Pod**: Automatically injects volume mounts based on annotations

## RBAC Requirements

### For Controller

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: shared-volume-controller
rules:
- apiGroups: ["sv.sharedvolume.io"]
  resources: ["sharedvolumes", "clustersharedvolumes"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["persistentvolumes", "persistentvolumeclaims", "services", "pods"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### For Users

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: shared-volume-user
rules:
- apiGroups: ["sv.sharedvolume.io"]
  resources: ["sharedvolumes"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```
