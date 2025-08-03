# Shared Volume Controller

[![Go Report Card](https://goreportcard.com/badge/github.com/sharedvolume/shared-volume-controller)](https://goreportcard.com/report/github.com/sharedvolume/shared-volume-controller)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/release/sharedvolume/shared-volume-controller.svg)](https://github.com/sharedvolume/shared-volume-controller/releases)

A Kubernetes operator that manages shared volumes with NFS server integration, enabling seamless data sharing across pods and namespaces.

## Features

- **SharedVolume**: Namespace-scoped shared volumes for data sharing within a namespace
- **ClusterSharedVolume**: Cluster-scoped shared volumes for cross-namespace data sharing
- **NFS Integration**: Automatic NFS server provisioning and management
- **Webhook Validation**: Built-in admission webhooks for resource validation
- **Data Synchronization**: Support for syncing data from various sources (SSH, Git, S3)
- **Pod Injection**: Automatic volume mounting in pods through admission webhooks

## Quick Start

### Prerequisites

- Kubernetes 1.19+
- kubectl configured to communicate with your cluster
- Cluster administrator privileges

### Installation

1. **Install the CRDs and operator:**

```bash
kubectl apply -f https://github.com/sharedvolume/shared-volume-controller/releases/latest/download/install.yaml
```

2. **Verify the installation:**

```bash
kubectl get pods -n shared-volume-controller-system
```

### Basic Usage

#### Creating a SharedVolume

```yaml
apiVersion: sv.sharedvolume.io/v1alpha1
kind: SharedVolume
metadata:
  name: my-shared-volume
  namespace: default
spec:
  capacity: "10Gi"
  accessModes:
    - ReadWriteMany
  storageClassName: "standard"
```

#### Creating a ClusterSharedVolume

```yaml
apiVersion: sv.sharedvolume.io/v1alpha1
kind: ClusterSharedVolume
metadata:
  name: my-cluster-shared-volume
spec:
  capacity: "20Gi"
  accessModes:
    - ReadWriteMany
  storageClassName: "standard"
```

#### Using in Pods

Add the annotation to your pod to automatically mount the shared volume:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  annotations:
    sv.sharedvolume.io/mount: "my-shared-volume:/data"
spec:
  containers:
  - name: app
    image: nginx
    volumeMounts:
    - name: shared-data
      mountPath: /data
```

## Architecture

The Shared Volume Controller consists of several components:

- **SharedVolume Controller**: Manages namespace-scoped shared volumes
- **ClusterSharedVolume Controller**: Manages cluster-scoped shared volumes
- **Pod Cleanup Controller**: Handles cleanup of resources when pods are deleted
- **Admission Webhooks**: Validates and mutates resources during creation
- **NFS Server Integration**: Integrates with the NFS Server Controller for storage provisioning

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `ENABLE_WEBHOOKS` | Enable/disable admission webhooks | `true` |
| `CONTROLLER_NAMESPACE` | Namespace for controller resources | `shared-volume-controller-system` |

### Command Line Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--metrics-bind-address` | Metrics server bind address | `:8080` |
| `--health-probe-bind-address` | Health probe bind address | `:8081` |
| `--leader-elect` | Enable leader election | `false` |
| `--controller-namespace` | Controller namespace | `shared-volume-controller` |

## Examples

Check the [examples](examples/) directory for more detailed usage examples:

- [Basic SharedVolume](examples/basic-shared-volume.yaml)
- [ClusterSharedVolume with Git sync](examples/cluster-shared-volume-git.yaml)
- [Pod with automatic volume mounting](examples/pod-with-shared-volume.yaml)

## Development

### Prerequisites

- Go 1.24+
- Docker
- kubectl
- kind or a Kubernetes cluster for testing

### Building from Source

```bash
# Clone the repository
git clone https://github.com/sharedvolume/shared-volume-controller.git
cd shared-volume-controller

# Build the binary
make build

# Build the Docker image
make docker-build

# Deploy to cluster
make deploy
```

### Running Tests

```bash
# Run unit tests
make test

# Run e2e tests
make test-e2e
```

### Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md) for details.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add some amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## API Reference

### SharedVolume

| Field | Type | Description |
|-------|------|-------------|
| `spec.capacity` | string | Storage capacity (e.g., "10Gi") |
| `spec.accessModes` | []string | Volume access modes |
| `spec.storageClassName` | string | Storage class name |
| `spec.source` | object | Data source configuration |

### ClusterSharedVolume

Similar to SharedVolume but cluster-scoped, allowing cross-namespace access.

For detailed API documentation, see the [API Reference](docs/api.md).

## Troubleshooting

### Common Issues

1. **Pod fails to start with volume mount errors**
   - Check if the SharedVolume is in "Ready" phase
   - Verify the NFS server is running
   - Check network connectivity between nodes

2. **Webhook admission failures**
   - Ensure webhook certificates are valid
   - Check webhook service is running
   - Verify RBAC permissions

3. **Volume synchronization issues**
   - Check source credentials and accessibility
   - Verify network policies allow outbound connections
   - Review controller logs for detailed error messages

### Logs

```bash
# Controller logs
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager

# Webhook logs  
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-webhook
```

## Roadmap

- [ ] Support for additional storage backends (Ceph, GlusterFS)
- [ ] Enhanced security with RBAC and network policies
- [ ] Performance monitoring and metrics
- [ ] Automatic backup and restore capabilities
- [ ] Integration with GitOps workflows

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Support

- 📖 [Documentation](docs/)
- 🐛 [Issue Tracker](https://github.com/sharedvolume/shared-volume-controller/issues)
- 💬 [Discussions](https://github.com/sharedvolume/shared-volume-controller/discussions)

## Acknowledgments

- Built with [Kubebuilder](https://kubebuilder.io/)
- Integrates with [NFS Server Controller](https://github.com/sharedvolume/nfs-server-controller)
- Inspired by the Kubernetes community