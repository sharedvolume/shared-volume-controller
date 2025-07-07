# ClusterSharedVolume

## Overview

`ClusterSharedVolume` is a cluster-scoped Custom Resource Definition (CRD) that extends the functionality of `SharedVolume` to work at the cluster level. It provides the same shared volume capabilities but operates across all namespaces in a Kubernetes cluster.

## Key Features

- **Cluster-scoped**: Can be accessed from any namespace in the cluster
- **Shared Controller Logic**: Reuses the same base controller logic as `SharedVolume`
- **Automatic NFS Server Management**: Creates and manages NFS servers automatically
- **Resource Management**: Handles PersistentVolumes, PersistentVolumeClaims, ReplicaSets, and Services

## Architecture

The `ClusterSharedVolumeReconciler` implements a simple delegation pattern:

```
ClusterSharedVolume (cluster-scoped)
    ↓
Creates SharedVolume with same name in operation namespace
    ↓
SharedVolumeReconciler processes the SharedVolume normally
    ↓
Creates: NFS Server, PV, PVC, ReplicaSet, Service
```

### Key Design Principles

1. **Simple Delegation**: CSV controller only creates/deletes SharedVolume resources
2. **No Labels/Management**: No special labels or skip logic needed
3. **Clean Separation**: CSV manages SharedVolume lifecycle, SV controller handles all resources
4. **Status Mirroring**: CSV status mirrors the corresponding SharedVolume status

## Usage

### Basic Example

```yaml
apiVersion: sv.sharedvolume.io/v1alpha1
kind: ClusterSharedVolume
metadata:
  name: my-cluster-shared-volume
spec:
  storage:
    capacity: "10Gi"
    storageClassName: "standard"
    accessModes:
      - ReadWriteMany
  mountPath: "/shared-data"
  syncInterval: "5m"
  syncEnabled: true
```

### Controller Namespace and Resource Management

Since `ClusterSharedVolume` is cluster-scoped but needs to create namespaced resources, the CSV controller creates a corresponding `SharedVolume` in the operation namespace: `shared-volume-controller-operation`.

**Resource Creation Flow:**
1. CSV controller creates SharedVolume in operation namespace with same name and owner reference to CSV
2. SharedVolume controller processes the SV normally and creates all underlying resources (NfsServer, PV, PVC, etc.)
3. CSV controller monitors SV status and mirrors it to CSV status
4. When CSV is deleted, Kubernetes automatically deletes the owned SharedVolume via garbage collection

**Benefits:**
- Simple and clean: no complex coordination between controllers
- No race conditions: only one controller manages each resource type
- Automatic cleanup: owner references ensure proper garbage collection
- Clear relationships: owner references show the CSV → SV relationship
- Easy to understand: CSV creates SV, SV creates everything else

This namespace is automatically created by the controller when the first ClusterSharedVolume is processed.

## Differences from SharedVolume

| Feature | SharedVolume | ClusterSharedVolume |
|---------|--------------|-------------------|
| Scope | Namespaced | Cluster |
| Resource Namespace | Same as CR | Operation namespace (`shared-volume-controller-operation`) |
| API Group | sv.sharedvolume.io | sv.sharedvolume.io |
| Short Name | sv | csv |

## Implementation Details

The `ClusterSharedVolumeReconciler`:

1. Converts `ClusterSharedVolume` to `SharedVolume` for compatibility
2. Uses the operation namespace (`shared-volume-controller-operation`) for all created resources
3. Automatically creates the operation namespace if it doesn't exist
4. Reuses all logic from `VolumeControllerBase`
5. Maintains separate finalizer management for cluster-scoped resources

## Sample Files

- See `config/samples/sv_v1alpha1_clustersharedvolume.yaml` for example usage
- CRD definition available at `config/crd/bases/sv.sharedvolume.io_clustersharedvolumes.yaml`
