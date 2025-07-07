# Webhook Test Examples

## Testing SharedVolume annotation (sharedvolume.sv)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: test-pod-sv
  namespace: default
  annotations:
    sharedvolume.sv: "my-shared-volume"
spec:
  containers:
  - name: test-container
    image: nginx
```

## Testing ClusterSharedVolume annotation (sharedvolume.csv)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: test-pod-csv
  namespace: default
  annotations:
    sharedvolume.csv: "my-cluster-shared-volume"
spec:
  containers:
  - name: test-container
    image: nginx
```

## Testing both annotations together

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: test-pod-both
  namespace: default
  annotations:
    sharedvolume.sv: "my-shared-volume"
    sharedvolume.csv: "my-cluster-shared-volume"
spec:
  containers:
  - name: test-container
    image: nginx
```

## Testing multiple volumes of each type

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: test-pod-multiple
  namespace: default
  annotations:
    sharedvolume.sv: "sv1,sv2,sv3"
    sharedvolume.csv: "csv1,csv2"
spec:
  containers:
  - name: test-container
    image: nginx
```

## How it works:

1. **SharedVolume (sharedvolume.sv)**: 
   - Looks for SharedVolume resources in the same namespace as the pod
   - Creates PV/PVC in the pod's namespace
   - Uses the SharedVolume's namespace for resource operations

2. **ClusterSharedVolume (sharedvolume.csv)**:
   - Looks for ClusterSharedVolume resources (cluster-scoped, no namespace)
   - Creates PV/PVC in the pod's namespace
   - Uses the static namespace "shared-volume-controller-operation" for resource operations

3. **Same Finalizer**: Both use `sharedvolume.sv/pod-cleanup` finalizer for cleanup

4. **Same Webhook Logic**: The webhook handles both types with the same cleanup and mounting logic
