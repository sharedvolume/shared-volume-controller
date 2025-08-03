# Troubleshooting Guide

This guide helps you diagnose and resolve common issues with the Shared Volume Controller.

## General Troubleshooting Steps

### 1. Check Controller Status

```bash
# Check if controller pods are running
kubectl get pods -n shared-volume-controller-system

# Check controller logs
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager

# Check resource status
kubectl get sharedvolumes -A
kubectl get clustersharedvolumes
```

### 2. Verify Permissions

```bash
# Check if the controller has proper RBAC permissions
kubectl auth can-i create sharedvolumes --as=system:serviceaccount:shared-volume-controller-system:shared-volume-controller-manager

# Check cluster roles and bindings
kubectl get clusterrole | grep shared-volume
kubectl get clusterrolebinding | grep shared-volume
```

### 3. Check Webhooks

```bash
# Check webhook configurations
kubectl get validatingwebhookconfiguration | grep shared-volume
kubectl get mutatingwebhookconfiguration | grep shared-volume

# Check webhook endpoints
kubectl get endpoints -n shared-volume-controller-system
```

## Common Issues and Solutions

### Issue 1: Controller Pod Not Starting

**Symptoms:**
- Controller pod is in `CrashLoopBackOff` or `Pending` state
- Error messages in pod logs

**Diagnosis:**
```bash
kubectl describe pod -n shared-volume-controller-system -l app.kubernetes.io/name=shared-volume-controller
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager --previous
```

**Common Causes and Solutions:**

1. **Insufficient RBAC permissions**
   ```bash
   # Apply the complete RBAC configuration
   kubectl apply -f config/rbac/
   ```

2. **Resource constraints**
   ```yaml
   # Increase resource limits in the deployment
   resources:
     limits:
       cpu: 500m
       memory: 512Mi
     requests:
       cpu: 100m
       memory: 128Mi
   ```

3. **Certificate issues**
   ```bash
   # Check webhook certificates
   kubectl get secret -n shared-volume-controller-system
   
   # Regenerate certificates if needed
   kubectl delete secret webhook-server-certs -n shared-volume-controller-system
   kubectl restart deployment shared-volume-controller-manager -n shared-volume-controller-system
   ```

### Issue 2: SharedVolume Stuck in Pending Phase

**Symptoms:**
- SharedVolume resource shows phase as "Pending"
- No PVC or PV created

**Diagnosis:**
```bash
kubectl describe sharedvolume <volume-name> -n <namespace>
kubectl get events -n <namespace> --sort-by=.metadata.creationTimestamp
```

**Common Causes and Solutions:**

1. **Storage class not found**
   ```bash
   # Check available storage classes
   kubectl get storageclass
   
   # Create or specify a valid storage class
   ```

2. **Insufficient storage capacity**
   ```bash
   # Check node storage capacity
   kubectl describe nodes
   
   # Reduce requested capacity or add more storage
   ```

3. **NFS server controller not available**
   ```bash
   # Check if NFS server controller is running
   kubectl get pods -n nfs-server-controller-system
   
   # Install NFS server controller if missing
   ```

### Issue 3: Pod Volume Mount Failures

**Symptoms:**
- Pods fail to start with volume mount errors
- "Unable to attach or mount volumes" errors

**Diagnosis:**
```bash
kubectl describe pod <pod-name> -n <namespace>
kubectl get pv,pvc -n <namespace>
kubectl get nfsserver -A
```

**Common Causes and Solutions:**

1. **NFS server not ready**
   ```bash
   # Check NFS server status
   kubectl get nfsserver -A
   kubectl describe nfsserver <nfs-server-name>
   
   # Wait for NFS server to be ready or troubleshoot NFS issues
   ```

2. **Network connectivity issues**
   ```bash
   # Test network connectivity from worker nodes
   kubectl run test-pod --image=busybox -it --rm -- nslookup <nfs-service-name>
   
   # Check network policies
   kubectl get networkpolicy -A
   ```

3. **Mount permission issues**
   ```bash
   # Check volume permissions
   kubectl exec -it <pod-name> -- ls -la /mount/path
   
   # Update security context if needed
   ```

### Issue 4: Webhook Admission Failures

**Symptoms:**
- Pod creation fails with webhook errors
- "Internal error occurred" messages

**Diagnosis:**
```bash
kubectl get validatingwebhookconfiguration shared-volume-controller-validating-webhook-configuration -o yaml
kubectl get mutatingwebhookconfiguration shared-volume-controller-mutating-webhook-configuration -o yaml
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager | grep webhook
```

**Common Causes and Solutions:**

1. **Webhook service not accessible**
   ```bash
   # Check webhook service
   kubectl get service -n shared-volume-controller-system
   kubectl get endpoints -n shared-volume-controller-system
   
   # Restart webhook service
   kubectl restart deployment shared-volume-controller-manager -n shared-volume-controller-system
   ```

2. **Certificate validation failures**
   ```bash
   # Check certificate validity
   kubectl get secret webhook-server-certs -n shared-volume-controller-system -o yaml
   
   # Regenerate certificates
   kubectl delete secret webhook-server-certs -n shared-volume-controller-system
   ```

3. **Webhook timeout**
   ```yaml
   # Increase webhook timeout in webhook configuration
   admissionReviewVersions: ["v1", "v1beta1"]
   timeoutSeconds: 30
   ```

### Issue 5: Data Synchronization Problems

**Symptoms:**
- Volume content not synchronized from source
- Sync errors in controller logs

**Diagnosis:**
```bash
kubectl describe sharedvolume <volume-name> -n <namespace>
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager | grep sync
```

**Common Causes and Solutions:**

1. **Invalid source credentials**
   ```bash
   # Check credential secret
   kubectl get secret <credential-secret> -n <namespace>
   
   # Update credentials
   kubectl create secret generic <credential-secret> \
     --from-literal=username=<username> \
     --from-literal=password=<password>
   ```

2. **Network access issues**
   ```bash
   # Test connectivity from controller pod
   kubectl exec -it -n shared-volume-controller-system deployment/shared-volume-controller-manager -- nslookup github.com
   
   # Check egress network policies
   ```

3. **Repository access permissions**
   ```bash
   # Verify repository URL and branch
   # Check if repository is public or credentials are correct
   ```

## Performance Issues

### Issue 6: Slow Volume Provisioning

**Symptoms:**
- SharedVolume takes long time to reach Ready phase
- Timeouts during volume creation

**Diagnosis:**
```bash
kubectl get events -n <namespace> --sort-by=.metadata.creationTimestamp
kubectl top nodes
kubectl top pods -n shared-volume-controller-system
```

**Solutions:**

1. **Increase controller resources**
   ```yaml
   resources:
     limits:
       cpu: 1000m
       memory: 1Gi
     requests:
       cpu: 500m
       memory: 512Mi
   ```

2. **Optimize storage class**
   ```yaml
   # Use faster storage class
   storageClassName: "fast-ssd"
   ```

3. **Scale controller replicas**
   ```yaml
   spec:
     replicas: 3
   ```

## Debug Mode

### Enable Debug Logging

```yaml
# Update controller deployment
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --zap-log-level=debug
        - --zap-development=true
```

### Collect Debug Information

```bash
# Create debug information bundle
mkdir debug-info
kubectl get all -n shared-volume-controller-system -o yaml > debug-info/controller-resources.yaml
kubectl get sharedvolumes -A -o yaml > debug-info/sharedvolumes.yaml
kubectl get clustersharedvolumes -o yaml > debug-info/clustersharedvolumes.yaml
kubectl get events -A --sort-by=.metadata.creationTimestamp > debug-info/events.log
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager > debug-info/controller.log
```

## Health Checks

### Controller Health Endpoints

```bash
# Port forward to health endpoints
kubectl port-forward -n shared-volume-controller-system deployment/shared-volume-controller-manager 8081:8081

# Check health
curl http://localhost:8081/healthz
curl http://localhost:8081/readyz

# Check metrics
kubectl port-forward -n shared-volume-controller-system deployment/shared-volume-controller-manager 8080:8080
curl http://localhost:8080/metrics
```

### Resource Monitoring

```bash
# Check resource usage
kubectl top pods -n shared-volume-controller-system
kubectl describe node | grep -A 5 "Allocated resources"

# Monitor volume usage
kubectl get pv | grep shared-volume
kubectl describe pv <pv-name>
```

## Recovery Procedures

### Recover from Controller Failure

1. **Check controller status**
   ```bash
   kubectl get pods -n shared-volume-controller-system
   ```

2. **Restart controller**
   ```bash
   kubectl restart deployment shared-volume-controller-manager -n shared-volume-controller-system
   ```

3. **Verify recovery**
   ```bash
   kubectl get sharedvolumes -A
   kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager
   ```

### Recover Orphaned Resources

```bash
# Find orphaned PVs
kubectl get pv | grep shared-volume

# Find orphaned PVCs
kubectl get pvc -A | grep shared-volume

# Clean up orphaned resources
kubectl delete pv <orphaned-pv-name>
kubectl delete pvc <orphaned-pvc-name> -n <namespace>
```

## Getting Help

If you can't resolve the issue:

1. **Check the documentation**: Review the [installation guide](installation.md) and [API reference](api.md)
2. **Search existing issues**: Look through [GitHub issues](https://github.com/sharedvolume/shared-volume-controller/issues)
3. **Create a bug report**: Include:
   - Kubernetes version
   - Controller version
   - Error messages and logs
   - Steps to reproduce
   - Debug information bundle
4. **Join the community**: Participate in [GitHub discussions](https://github.com/sharedvolume/shared-volume-controller/discussions)

## Useful Commands Reference

```bash
# Quick status check
kubectl get pods,sharedvolumes,clustersharedvolumes -A

# Controller logs
kubectl logs -f -n shared-volume-controller-system deployment/shared-volume-controller-manager

# Events across all namespaces
kubectl get events -A --sort-by=.metadata.creationTimestamp

# Resource descriptions
kubectl describe sharedvolume <name> -n <namespace>
kubectl describe clustersharedvolume <name>

# Cleanup test resources
kubectl delete sharedvolume --all -n <namespace>
kubectl delete clustersharedvolume --all
```
