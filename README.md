# Shared Volume Controller

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/sharedvolume/shared-volume-controller)](https://goreportcard.com/report/github.com/sharedvolume/shared-volume-controller)
[![Docker Pulls](https://img.shields.io/docker/pulls/sharedvolume/shared-volume-controller)](https://hub.docker.com/r/sharedvolume/shared-volume-controller)
[![Release](https://img.shields.io/github/release/sharedvolume/shared-volume-controller.svg)](https://github.com/sharedvolume/shared-volume-controller/releases)

A Kubernetes operator that manages shared volumes with NFS server integration, enabling seamless data sharing across pods and namespaces with automatic data synchronization from various sources.

## Features

- **SharedVolume**: Namespace-scoped shared volumes for data sharing within a namespace
- **ClusterSharedVolume**: Cluster-scoped shared volumes for cross-namespace data sharing
- **NFS Integration**: Automatic NFS server provisioning and management via [NFS Server Controller](https://github.com/sharedvolume/nfs-server-controller)
- **Multi-Source Sync**: Support for syncing data from SSH, Git, HTTP, and S3 sources
- **Admission Webhooks**: Built-in validation and mutation webhooks for resource management
- **Pod Injection**: Automatic volume mounting in pods through admission webhooks
- **Real-time Sync**: Configurable sync intervals for keeping data up-to-date
- **Storage Flexibility**: Support for various storage classes and access modes

## Quick Start

### Prerequisites

- Kubernetes cluster (v1.19+)
- kubectl configured to access your cluster
- Cluster admin permissions
- [NFS Server Controller](https://github.com/sharedvolume/nfs-server-controller) (installed automatically)

### Installation

1. **Install the CRDs and operator:**
   ```bash
   kubectl apply -f https://github.com/sharedvolume/shared-volume-controller/releases/latest/download/install.yaml
   ```

2. **Verify the installation:**
   ```bash
   kubectl get deployment -n shared-volume-controller-system
   kubectl get crd sharedvolumes.sv.sharedvolume.io clustersharedvolumes.sv.sharedvolume.io
   ```

### Creating a Shared Volume

#### Basic SharedVolume with local storage:

```yaml
apiVersion: sv.sharedvolume.io/v1alpha1
kind: SharedVolume
metadata:
  name: my-shared-volume
  namespace: default
spec:
  mountPath: "/shared"
  storage:
    capacity: "10Gi"
    accessMode: "ReadWrite"
  storageClassName: "standard"
```

#### SharedVolume with Git synchronization:

```yaml
apiVersion: sv.sharedvolume.io/v1alpha1
kind: SharedVolume
metadata:
  name: git-shared-volume
  namespace: default
spec:
  mountPath: "/git-data"
  storage:
    capacity: "5Gi"
  storageClassName: "fast-ssd"
  source:
    git:
      url: "https://github.com/example/data-repo.git"
      branch: "main"
  syncInterval: "5m"
```

Apply the configuration:
```bash
kubectl apply -f shared-volume.yaml
```

### Using Shared Volumes in Pods

#### Automatic mounting with annotations:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: app-pod
  namespace: default
  annotations:
    sv.sharedvolume.io/mount: "my-shared-volume:/app/shared"
spec:
  containers:
  - name: app
    image: nginx
    # Volume will be automatically mounted at /app/shared
```

#### Manual volume mounting:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: manual-pod
  namespace: default
spec:
  containers:
  - name: app
    image: nginx
    volumeMounts:
    - name: shared-data
      mountPath: /data
  volumes:
  - name: shared-data
    nfs:
      server: my-shared-volume-nfs.default.svc.cluster.local
      path: /shared
```

## Configuration

### SharedVolume Spec

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| `mountPath` | string | Mount path inside the NFS server | Yes |
| `storage.capacity` | string | Storage capacity (e.g., "10Gi") | Yes |
| `storage.accessMode` | string | Access mode (ReadWrite/ReadOnly) | No |
| `storageClassName` | string | StorageClass name for dynamic provisioning | No |
| `source` | object | Data source configuration (Git, SSH, HTTP, S3) | No |
| `syncInterval` | string | Sync interval (e.g., "5m", "1h") | No |
| `syncTimeout` | string | Sync operation timeout (e.g., "120s") | No |
| `nfsServer` | object | Custom NFS server configuration | No |

### ClusterSharedVolume Spec

Similar to SharedVolume but cluster-scoped, allowing cross-namespace access.

### Data Source Types

#### Git Source:
```yaml
source:
  git:
    url: "https://github.com/example/repo.git"
    branch: "main"
    user: "username"
    passwordFromSecret:
      name: "git-credentials"
      key: "password"
```

#### SSH Source:
```yaml
source:
  ssh:
    host: "server.example.com"
    port: 22
    user: "ubuntu"
    path: "/remote/data"
    privateKeyFromSecret:
      name: "ssh-key"
      key: "private-key"
```

#### S3 Source:
```yaml
source:
  s3:
    endpointUrl: "https://s3.amazonaws.com"
    bucketName: "my-bucket"
    path: "data/"
    region: "us-west-2"
    accessKeyFromSecret:
      name: "s3-credentials"
      key: "access-key"
    secretKeyFromSecret:
      name: "s3-credentials"
      key: "secret-key"
```

#### HTTP Source:
```yaml
source:
  http:
    url: "https://example.com/data.zip"
```

## Examples

### ClusterSharedVolume for Multi-Namespace Access

```yaml
apiVersion: sv.sharedvolume.io/v1alpha1
kind: ClusterSharedVolume
metadata:
  name: shared-configs
spec:
  mountPath: "/configs"
  storage:
    capacity: "2Gi"
  storageClassName: "standard"
  source:
    git:
      url: "https://github.com/company/k8s-configs.git"
      branch: "production"
  syncInterval: "10m"
```

### Using ClusterSharedVolume in different namespaces:

```yaml
# In namespace "app1"
apiVersion: v1
kind: Pod
metadata:
  name: app1-pod
  namespace: app1
  annotations:
    sv.sharedvolume.io/mount: "shared-configs:/etc/configs"
spec:
  containers:
  - name: app
    image: myapp:v1
---
# In namespace "app2"  
apiVersion: v1
kind: Pod
metadata:
  name: app2-pod
  namespace: app2
  annotations:
    sv.sharedvolume.io/mount: "shared-configs:/etc/configs"
spec:
  containers:
  - name: app
    image: myapp:v2
```

## Development

### Prerequisites

- Go 1.24+
- Docker
- kubectl
- Kind (for local testing)

### Building from Source

1. **Clone the repository:**
   ```bash
   git clone https://github.com/sharedvolume/shared-volume-controller.git
   cd shared-volume-controller
   ```

2. **Build the manager:**
   ```bash
   make build
   ```

3. **Run tests:**
   ```bash
   make test
   ```

4. **Build Docker image:**
   ```bash
   make docker-build IMG=shared-volume-controller:dev
   ```

### Local Development

1. **Install CRDs:**
   ```bash
   make install
   ```

2. **Run the controller locally:**
   ```bash
   make run
   ```

3. **Deploy to cluster:**
   ```bash
   make deploy IMG=shared-volume-controller:dev
   ```

### Contributing

We welcome contributions! Please see our [Contributing Guidelines](CONTRIBUTING.md) for details.

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Ensure all tests pass
6. Submit a pull request

## Architecture

The Shared Volume Controller consists of:

- **SharedVolume Controller**: Manages namespace-scoped shared volumes and their lifecycle
- **ClusterSharedVolume Controller**: Manages cluster-scoped shared volumes for cross-namespace access
- **Pod Cleanup Controller**: Handles cleanup of resources when pods are deleted
- **Admission Webhooks**: Validates and mutates SharedVolume/ClusterSharedVolume resources and pods
- **Volume Syncer Integration**: Integrates with the [Volume Syncer](https://github.com/sharedvolume/volume-syncer) for data synchronization
- **NFS Server Integration**: Works with [NFS Server Controller](https://github.com/sharedvolume/nfs-server-controller) for storage provisioning

### Resource Lifecycle

1. **Creation**: User creates SharedVolume/ClusterSharedVolume
2. **Validation**: Admission webhooks validate the resource
3. **NFS Provisioning**: Controller creates NFS server via NFS Server Controller
4. **Storage Setup**: PVC and storage resources are created
5. **Data Sync**: If source is specified, volume syncer begins syncing data
6. **Ready State**: Volume becomes available for pod mounting
7. **Pod Injection**: Admission webhooks automatically mount volumes in annotated pods

## Monitoring

Check resource status:

```bash
# List SharedVolumes
kubectl get sharedvolumes
kubectl get sv  # shortname

# List ClusterSharedVolumes  
kubectl get clustersharedvolumes
kubectl get csv  # shortname

# Detailed status
kubectl describe sharedvolume my-shared-volume
```

Example output:
```
NAME                PHASE   NFS ADDRESS
my-shared-volume    Ready   my-shared-volume-nfs.default.svc.cluster.local
git-shared-volume   Ready   git-shared-volume-nfs.default.svc.cluster.local
```

## Troubleshooting

### Common Issues

1. **SharedVolume stuck in Pending phase:**
   - Check NFS Server Controller is installed and running
   - Verify storage class exists: `kubectl get storageclass`
   - Check controller logs for detailed errors

2. **Data synchronization failures:**
   - Verify source credentials are correct and accessible
   - Check network policies allow outbound connections
   - Review volume syncer logs: `kubectl logs -l app=volume-syncer`

3. **Pod mounting issues:**
   - Ensure pods have proper annotations for automatic mounting
   - Verify SharedVolume is in Ready phase
   - Check NFS client utilities in pod images

4. **Webhook admission failures:**
   - Ensure webhook certificates are valid and not expired
   - Check webhook service is running and accessible
   - Verify RBAC permissions for webhook operations

### Logs

```bash
# Controller logs
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager

# Webhook logs  
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-webhook

# NFS server logs
kubectl logs -l app=nfs-server -A
```

### Debug Commands

```bash
# Check resource events
kubectl describe sharedvolume my-shared-volume

# List related resources
kubectl get nfsservers,pvc,services -l sharedvolume.io/managed-by=shared-volume-controller

# Check webhook configuration
kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration
```

## Security Considerations

- SharedVolumes are namespace-scoped and respect RBAC permissions
- ClusterSharedVolumes require cluster-level permissions to create
- Sensitive source credentials should be stored in Kubernetes secrets
- NFS traffic is unencrypted by default - consider network policies for security
- Volume syncer pods may require elevated permissions for certain source types
- Regularly rotate credentials and update source configurations

## Performance Tuning

- Adjust `syncInterval` based on data change frequency
- Use appropriate `syncTimeout` for large data transfers
- Choose optimal storage classes for your workload requirements
- Consider resource limits for volume syncer pods
- Monitor NFS server performance and scale replicas if needed

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

- **Issues**: Report bugs and feature requests on [GitHub Issues](https://github.com/sharedvolume/shared-volume-controller/issues)
- **Discussions**: Join community discussions on [GitHub Discussions](https://github.com/sharedvolume/shared-volume-controller/discussions)
- **Documentation**: Detailed docs in the [docs/](docs/) directory
- **Security**: Report security vulnerabilities privately to security@sharedvolume.io

## Related Projects

- [NFS Server Controller](https://github.com/sharedvolume/nfs-server-controller) - Manages NFS servers in Kubernetes
- [Volume Syncer](https://github.com/sharedvolume/volume-syncer) - Handles data synchronization from various sources
- [NFS Server Image](https://github.com/sharedvolume/nfs-server-image) - Container image for NFS servers

## Acknowledgments

Built with [Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder) and inspired by the Kubernetes community's best practices for operators and volume management.