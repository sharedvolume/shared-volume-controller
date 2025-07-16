# Test ClusterSharedVolume Simple Delegation

This document outlines how to test that the ClusterSharedVolume controller properly creates SharedVolumes and lets the SV controller handle all resource management.

## Test Steps

### 1. Deploy the Controller

```bash
# Build and deploy
make docker-build docker-push IMG=your-registry/shared-volume-controller:latest
make deploy IMG=your-registry/shared-volume-controller:latest
```

### 2. Create a ClusterSharedVolume

```bash
kubectl apply -f - <<EOF
apiVersion: sv.sharedvolume.io/v1alpha1
kind: ClusterSharedVolume
metadata:
  name: test-csv
spec:
  storage:
    capacity: "1Gi"
    storageClassName: "standard"
    accessModes:
      - ReadWriteMany
  mountPath: "/shared-data"
EOF
```

### 3. Verify Simple Delegation Works

#### Check SharedVolume Creation
```bash
# Verify SharedVolume is created in operation namespace with same name
kubectl get sharedvolume -n shared-volume-controller

# Should show: test-csv SharedVolume
```

#### Check NfsServer Creation
```bash
# Should see exactly ONE NfsServer created by SharedVolume controller
kubectl get nfsserver -n shared-volume-controller

# Check the NfsServer is owned by SharedVolume
kubectl get nfsserver test-csv-nfs -n shared-volume-controller -o yaml | grep ownerReferences -A 10
```

#### Monitor Controller Logs
```bash
# Check CSV controller logs - should only show SharedVolume creation
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager | grep -i clustershared

# Check SV controller logs - should show normal processing
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager | grep -i "test-csv"
```

### 4. Verify Status Sync

```bash
# Check CSV status mirrors SV status
kubectl get clustersharedvolume test-csv -o yaml | grep -A 10 status:
kubectl get sharedvolume test-csv -n shared-volume-controller -o yaml | grep -A 10 status:
```

### Expected Results

1. **One SharedVolume** created in operation namespace with same name as CSV
2. **One NfsServer** created and owned by the SharedVolume
3. **Normal SV controller processing** - no special handling or skipping
4. **CSV status** mirrors SharedVolume status
5. **No race conditions** - clean separation of responsibilities

## Cleanup

```bash
kubectl delete clustersharedvolume test-csv
# Verify all resources are cleaned up
kubectl get all -n shared-volume-controller
```
