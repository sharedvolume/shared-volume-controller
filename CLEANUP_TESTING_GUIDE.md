# Comprehensive Cleanup System - Testing Guide

## Overview

The shared volume controller now has a comprehensive cleanup system that ensures complete deletion of all related resources when you delete pods or SharedVolumes.

## What's Fixed

### 1. Pod Deletion Cleanup
When you delete a pod that uses SharedVolumes:
- ✅ Pod gets force deleted (removes finalizers if stuck)
- ✅ Related PVCs get force deleted (both pod-specific and orphaned ones)
- ✅ Related PVs get force deleted (both pod-specific and orphaned ones)
- ✅ Automatic detection of stuck resources with finalizer removal

### 2. SharedVolume Deletion Cleanup
When you delete a SharedVolume:
- ✅ All pods using the SharedVolume get force deleted
- ✅ ReplicaSets get deleted
- ✅ Services get deleted
- ✅ All related PVCs get deleted (main + namespace-specific)
- ✅ All related PVs get deleted (main + namespace-specific)
- ✅ NFS Servers get deleted
- ✅ Comprehensive search across all namespaces for related resources

### 3. Force Delete Utility
Manual cleanup tool for emergency situations:
- ✅ Individual resource force deletion
- ✅ SharedVolume-based cleanup (cleans all related resources)
- ✅ Nuclear option to clean ALL stuck resources
- ✅ Status checking for stuck resources

## Testing Instructions

### Test 1: Pod Deletion Cleanup

```bash
# 1. Create a pod with SharedVolume
kubectl apply -f config/samples/example-pod/alpine.yaml

# 2. Verify resources are created
kubectl get pods,pvc,pv | grep alpine

# 3. Delete the pod
kubectl delete pod alpine-sample-pod-1

# 4. Verify everything is cleaned up (should return empty)
kubectl get pods,pvc,pv | grep alpine
```

### Test 2: SharedVolume Deletion Cleanup

```bash
# 1. Create a SharedVolume
kubectl apply -f config/samples/sv_v1alpha1_sharedvolume.yaml

# 2. Wait for resources to be created
kubectl get sharedvolume,rs,svc,pvc,pv

# 3. Delete the SharedVolume
kubectl delete sharedvolume sharedvolume-sample

# 4. Verify all related resources are cleaned up
kubectl get sharedvolume,rs,svc,pvc,pv | grep sharedvolume-sample
```

### Test 3: Force Delete Utility

```bash
# Check for stuck resources
./hack/force-delete.sh status

# Force delete specific resources
./hack/force-delete.sh pod <pod-name> <namespace>
./hack/force-delete.sh pvc <pvc-name> <namespace>
./hack/force-delete.sh pv <pv-name>

# Clean up all resources for a SharedVolume
./hack/force-delete.sh sharedvolume <sharedvolume-name> <namespace>

# Nuclear option - clean ALL stuck resources (use with caution)
./hack/force-delete.sh cleanup-all
```

## Key Improvements

### 1. Enhanced Controllers

**Pod Cleanup Controller:**
- Immediate finalizer removal for already terminating resources
- Orphaned resource detection and cleanup
- Comprehensive search patterns for related PVCs/PVs
- Retry logic with timeouts

**SharedVolume Controller:**
- Cross-namespace pod discovery and cleanup
- Pattern-based resource matching for cleanup
- Modular cleanup functions for better maintainability
- Force deletion integrated into normal workflow

### 2. Robust Force Deletion

**Smart Finalizer Handling:**
```go
// Try immediate finalizer removal if already terminating
if resource.DeletionTimestamp != nil && len(resource.Finalizers) > 0 {
    resource.Finalizers = []string{}
    r.Update(ctx, resource)
    return nil
}
```

**Comprehensive Resource Discovery:**
- Searches by naming patterns
- Annotation-based discovery
- Cross-namespace searching
- Multiple pattern matching

### 3. Enhanced RBAC

```yaml
# Added finalizer update permissions
- apiGroups: [""]
  resources: ["persistentvolumeclaims/finalizers", "persistentvolumes/finalizers", "pods/finalizers"]
  verbs: ["update"]
```

## Troubleshooting

### If Resources Are Still Stuck

1. **Check the status:**
   ```bash
   ./hack/force-delete.sh status
   ```

2. **Try manual force deletion:**
   ```bash
   ./hack/force-delete.sh sharedvolume <name> <namespace>
   ```

3. **Nuclear option (emergency only):**
   ```bash
   ./hack/force-delete.sh cleanup-all
   ```

### Common Issues

**Issue:** Pod won't delete
**Solution:** Controller automatically removes finalizers, or use force delete script

**Issue:** PVC/PV stuck in terminating
**Solution:** Controller removes protection finalizers automatically

**Issue:** Multiple related resources scattered
**Solution:** Use SharedVolume-based cleanup to find and delete all related resources

## Deployment

1. **Build and deploy the updated controller:**
   ```bash
   make build
   make deploy
   ```

2. **Verify the enhanced RBAC permissions are applied:**
   ```bash
   kubectl describe clusterrole manager-role | grep finalizers
   ```

3. **Test with your existing resources:**
   ```bash
   # Create test resources
   kubectl apply -f config/samples/example-pod/alpine.yaml
   
   # Delete and verify cleanup
   kubectl delete pod alpine-sample-pod-1
   kubectl get pods,pvc,pv | grep alpine  # Should be empty
   ```

## Monitoring

The controllers now provide enhanced logging:

```bash
# Watch controller logs for cleanup activities
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager -f

# Look for these log messages:
# - "Starting comprehensive cleanup"
# - "Force deleting pod/pvc/pv"
# - "Successfully removed finalizers"
# - "Completed comprehensive cleanup"
```

Your cleanup issues should now be completely resolved! 🎉
