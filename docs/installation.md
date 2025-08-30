# Installation Guide

This guide provides detailed instructions for installing and configuring the Shared Volume Controller in your Kubernetes cluster.

## Prerequisites

- Kubernetes cluster 1.19 or later
- kubectl configured to communicate with your cluster
- Cluster administrator privileges
- At least 2GB of available storage for the controller components

## Installation Methods

### Method 1: Quick Install (Recommended)

Install the latest release using the provided manifests:

```bash
kubectl apply -f https://github.com/sharedvolume/shared-volume-controller/releases/latest/download/install.yaml
```

### Method 2: Helm Chart (Coming Soon)

```bash
helm repo add shared-volume https://charts.sharedvolume.io
helm install shared-volume-controller shared-volume/shared-volume-controller
```

### Method 3: Manual Installation

1. **Clone the repository:**
   ```bash
   git clone https://github.com/sharedvolume/shared-volume-controller.git
   cd shared-volume-controller
   ```

2. **Install CRDs:**
   ```bash
   make install
   ```

3. **Deploy the controller:**
   ```bash
   make deploy
   ```

## Configuration

### Environment Variables

Configure the controller using environment variables in the deployment:

```yaml
env:
- name: ENABLE_WEBHOOKS
  value: "true"
- name: CONTROLLER_NAMESPACE
  value: "shared-volume-controller-system"
- name: LOG_LEVEL
  value: "info"
```

### Command Line Flags

Override default settings using command line flags:

```yaml
args:
- --metrics-bind-address=:8080
- --health-probe-bind-address=:8081
- --leader-elect=true
- --controller-namespace=shared-volume-controller-system
```

### Storage Classes

Configure storage classes for different performance requirements:

```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: shared-volume-fast
provisioner: kubernetes.io/aws-ebs
parameters:
  type: gp3
  iops: "3000"
allowVolumeExpansion: true
```

## Verification

### Check Controller Status

```bash
# Check if the controller is running
kubectl get pods -n shared-volume-controller-system

# Check controller logs
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager
```

### Verify CRDs

```bash
# List installed CRDs
kubectl get crd | grep sharedvolume

# Expected output:
# clustersharedvolumes.sv.sharedvolume.io
# sharedvolumes.sv.sharedvolume.io
```

### Test Installation

Create a test SharedVolume:

```bash
cat <<EOF | kubectl apply -f -
apiVersion: sv.sharedvolume.io/v1alpha1
kind: SharedVolume
metadata:
  name: test-volume
  namespace: default
spec:
  capacity: "1Gi"
  accessModes:
    - ReadWriteMany
  storageClassName: "standard"
EOF
```

Check the status:

```bash
kubectl get sharedvolume test-volume -o yaml
```

## Security Configuration

### RBAC Setup

The controller requires specific RBAC permissions. These are automatically created during installation, but for custom setups:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: shared-volume-controller-manager
rules:
- apiGroups: ["sv.sharedvolume.io"]
  resources: ["sharedvolumes", "clustersharedvolumes"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["persistentvolumes", "persistentvolumeclaims", "services", "pods"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### Network Policies

For enhanced security, apply network policies:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: shared-volume-controller-netpol
  namespace: shared-volume-controller-system
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: shared-volume-controller
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector: {}
    ports:
    - protocol: TCP
      port: 9443
  egress:
  - {}
```

### Pod Security Standards

Apply Pod Security Standards for the controller namespace:

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: shared-volume-controller-system
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

## Advanced Configuration

### Custom Certificates

For production environments, use custom certificates for webhooks:

```bash
# Create certificate secret
kubectl create secret tls webhook-certs \
  --cert=webhook.crt \
  --key=webhook.key \
  -n shared-volume-controller-system
```

Update the deployment to use custom certificates:

```yaml
args:
- --webhook-cert-path=/etc/certs
- --webhook-cert-name=tls.crt
- --webhook-cert-key=tls.key
volumeMounts:
- name: webhook-certs
  mountPath: /etc/certs
  readOnly: true
volumes:
- name: webhook-certs
  secret:
    secretName: webhook-certs
```

### Resource Limits

Configure appropriate resource limits for production:

```yaml
resources:
  limits:
    cpu: 500m
    memory: 512Mi
  requests:
    cpu: 100m
    memory: 128Mi
```

### Multiple Replicas

For high availability, run multiple controller replicas:

```yaml
spec:
  replicas: 3
  template:
    spec:
      containers:
      - name: manager
        args:
        - --leader-elect=true
```

## Troubleshooting

### Common Issues

1. **Controller not starting:**
   ```bash
   kubectl describe pod -n shared-volume-controller-system
   kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager
   ```

2. **Webhook failures:**
   ```bash
   kubectl get validatingwebhookconfiguration
   kubectl get mutatingwebhookconfiguration
   ```

3. **RBAC issues:**
   ```bash
   kubectl auth can-i create sharedvolumes --as=system:serviceaccount:shared-volume-controller-system:shared-volume-controller-manager
   ```

### Log Levels

Increase log verbosity for debugging:

```yaml
args:
- --zap-log-level=debug
- --zap-development=true
```

### Health Checks

Monitor controller health:

```bash
# Health check endpoint
kubectl port-forward -n shared-volume-controller-system deployment/shared-volume-controller-manager 8081:8081
curl http://localhost:8081/healthz

# Metrics endpoint
kubectl port-forward -n shared-volume-controller-system deployment/shared-volume-controller-manager 8080:8080
curl http://localhost:8080/metrics
```

## Upgrading

### From Previous Versions

1. **Check compatibility:**
   ```bash
   kubectl version
   ```

2. **Backup existing resources:**
   ```bash
   kubectl get sharedvolumes -A -o yaml > sharedvolumes-backup.yaml
   kubectl get clustersharedvolumes -o yaml > clustersharedvolumes-backup.yaml
   ```

3. **Upgrade the controller:**
   ```bash
   kubectl apply -f https://github.com/sharedvolume/shared-volume-controller/releases/latest/download/install.yaml
   ```

4. **Verify upgrade:**
   ```bash
   kubectl get pods -n shared-volume-controller-system
   kubectl get sharedvolumes -A
   ```

## Uninstallation

### Complete Removal

1. **Delete all SharedVolume resources:**
   ```bash
   kubectl delete sharedvolumes --all --all-namespaces
   kubectl delete clustersharedvolumes --all
   ```

2. **Uninstall the controller:**
   ```bash
   kubectl delete -f https://github.com/sharedvolume/shared-volume-controller/releases/latest/download/install.yaml
   ```

3. **Remove CRDs (optional):**
   ```bash
   kubectl delete crd sharedvolumes.sv.sharedvolume.io
   kubectl delete crd clustersharedvolumes.sv.sharedvolume.io
   ```

## Support

For installation issues:
- Check the [troubleshooting guide](../docs/troubleshooting.md)
- Open an issue on [GitHub](https://github.com/sharedvolume/shared-volume-controller/issues)
- Join our [community discussions](https://github.com/sharedvolume/shared-volume-controller/discussions)
