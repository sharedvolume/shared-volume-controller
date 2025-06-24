# Webhook Implementation

## Overview

The SharedVolume controller includes a mutating admission webhook that automatically mounts SharedVolumes to pods based on annotations. This eliminates the need to manually configure PersistentVolumes, PersistentVolumeClaims, and volume mounts in your pod specifications.

## How It Works

When you create a pod with the appropriate annotations, the webhook:

1. **Detects** the SharedVolume annotation pattern: `sharedvolume.io/sv/{shared_volume_name}: "true"`
2. **Fetches** the corresponding SharedVolume resource from the same namespace
3. **Creates** a PersistentVolume and PersistentVolumeClaim if they don't already exist
4. **Mounts** the volume to all containers in the pod

## Annotation Format

```yaml
metadata:
  annotations:
    sharedvolume.io/sv/{shared_volume_name}: "true"
```

Where `{shared_volume_name}` is the name of the SharedVolume resource in the same namespace.

## Generated Resources

### PersistentVolume

The webhook creates a PV with the following specification:

```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: {SharedVolume.Spec.ReferenceValue}-{PodNamespace}
spec:
  capacity:
    storage: {SharedVolume.Spec.Storage.Capacity}
  accessModes:
    - ReadWriteMany
  persistentVolumeReclaimPolicy: Delete
  mountOptions:
    - nfsvers=4.1
  csi:
    driver: nfs.csi.k8s.io
    volumeHandle: {SharedVolume.Status.NfsServerAddress}/share##
    volumeAttributes:
      server: {SharedVolume.Status.NfsServerAddress}
      share: {SharedVolume.Spec.NfsServer.Path}{SharedVolume.Name}-{SharedVolume.Namespace}
```

### PersistentVolumeClaim

The webhook creates a PVC with the following specification:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: {SharedVolume.Spec.ReferenceValue}-{PodNamespace}
  namespace: {PodNamespace}
spec:
  accessModes:
    - ReadWriteMany
  resources:
    requests:
      storage: {SharedVolume.Spec.Storage.Capacity}
  storageClassName: {SharedVolume.Spec.StorageClassName} # if specified
```

### Volume Mount

The webhook adds the following to each container in the pod:

```yaml
volumeMounts:
  - mountPath: {SharedVolume.Spec.MountPath}
    name: {SharedVolume.Spec.ReferenceValue}
    readOnly: {SharedVolume.Spec.Storage.AccessMode == "ReadOnly"}
```

And adds this volume to the pod spec:

```yaml
volumes:
  - name: {SharedVolume.Spec.ReferenceValue}
    persistentVolumeClaim:
      claimName: {SharedVolume.Spec.ReferenceValue}-{PodNamespace}
```

## Example Usage

1. **Create a SharedVolume resource:**

```yaml
apiVersion: sv.sharedvolume.io/v1alpha1
kind: SharedVolume
metadata:
  name: my-shared-volume
  namespace: default
spec:
  mountPath: /mnt/shared
  referenceValue: my-shared-ref
  storage:
    capacity: 10Gi
    accessMode: ReadWrite
  nfsServer:
    path: /exports
  # ... other specifications
```

2. **Create a pod with the annotation:**

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  namespace: default
  annotations:
    sharedVolume.io/sv/my-shared-volume: "true"
spec:
  containers:
    - name: my-container
      image: nginx:1.21
      # Volume mounts will be automatically added by the webhook
```

3. **The webhook automatically:**
   - Creates PV named `my-shared-ref-default`
   - Creates PVC named `my-shared-ref-default` 
   - Mounts the volume at `/mnt/shared` in the container

## Multi-Volume Support

You can mount multiple SharedVolumes to a single pod:

```yaml
metadata:
  annotations:
    sharedvolume.io/sv/volume1: "true"
    sharedvolume.io/sv/volume2: "true"
    sharedvolume.io/sv/volume3: "true"
```

## Resource Reuse

The webhook is smart about resource reuse:
- If a PV with the same name already exists, it reuses it
- If a PVC with the same name already exists in the namespace, it reuses it
- This allows multiple pods to share the same underlying storage

## Error Handling

- If the SharedVolume resource doesn't exist, the webhook will reject the pod creation
- If there are issues creating PV/PVC resources, the webhook will return an error
- Check the webhook logs for detailed error information

## Webhook Configuration

The webhook is configured to intercept pod creation and modification events. Make sure the webhook is properly deployed and the TLS certificates are configured correctly for the webhook to function.
