# Development Guide

This guide provides information for developers who want to contribute to or extend the Shared Volume Controller.

## Development Environment Setup

### Prerequisites

- Go 1.24 or later
- Docker 20.10 or later
- kubectl configured with a development cluster
- kind or minikube for local testing
- make utility

### Clone and Setup

```bash
# Clone the repository
git clone https://github.com/sharedvolume/shared-volume-controller.git
cd shared-volume-controller

# Install dependencies
go mod download

# Install development tools
make install-tools
```

### Local Development Cluster

#### Using kind

```bash
# Create a kind cluster
kind create cluster --name shared-volume-dev --config hack/kind-config.yaml

# Set kubectl context
kubectl cluster-info --context kind-shared-volume-dev
```

#### Using minikube

```bash
# Start minikube
minikube start --driver=docker --kubernetes-version=v1.28.0

# Enable required addons
minikube addons enable default-storageclass
minikube addons enable storage-provisioner
```

## Project Structure

```
shared-volume-controller/
├── api/                    # API definitions (CRDs)
│   └── v1alpha1/          # API version v1alpha1
├── cmd/                   # Main applications
│   └── main.go           # Controller entry point
├── config/               # Kubernetes manifests
│   ├── crd/             # Custom Resource Definitions
│   ├── rbac/            # RBAC configurations
│   ├── manager/         # Controller deployment
│   └── webhook/         # Webhook configurations
├── internal/            # Private application code
│   ├── controller/      # Controller implementations
│   └── webhook/         # Webhook implementations
├── examples/            # Example manifests
├── docs/                # Documentation
├── hack/                # Scripts and utilities
└── test/                # Test files
```

## Development Workflow

### 1. Code Generation

The project uses Kubebuilder for code generation:

```bash
# Generate CRD manifests
make manifests

# Generate code (DeepCopy methods)
make generate

# Update vendor dependencies
go mod tidy && go mod vendor
```

### 2. Building

```bash
# Build the binary
make build

# Build and push Docker image
make docker-build docker-push IMG=<your-registry>/shared-volume-controller:dev
```

### 3. Testing

```bash
# Run unit tests
make test

# Run tests with coverage
make test-coverage

# Run integration tests
make test-integration

# Run end-to-end tests
make test-e2e
```

### 4. Local Development

#### Running Controller Locally

```bash
# Install CRDs
make install

# Run controller locally (outside cluster)
make run

# Or run against remote cluster
KUBECONFIG=~/.kube/config make run
```

#### Running in Cluster

```bash
# Deploy to development cluster
make deploy IMG=<your-registry>/shared-volume-controller:dev

# Check deployment
kubectl get pods -n shared-volume-controller-system

# View logs
kubectl logs -f -n shared-volume-controller-system deployment/shared-volume-controller-manager
```

## Making Changes

### Adding New API Fields

1. **Modify the API types** in `api/v1alpha1/`:
   ```go
   // Add new field to spec
   type SharedVolumeSpec struct {
       // existing fields...
       NewField string `json:"newField,omitempty"`
   }
   ```

2. **Generate code**:
   ```bash
   make generate manifests
   ```

3. **Update controller logic** in `internal/controller/`:
   ```go
   // Handle new field in reconcile logic
   if sharedVolume.Spec.NewField != "" {
       // implement logic
   }
   ```

4. **Add tests**:
   ```go
   func TestNewField(t *testing.T) {
       // test implementation
   }
   ```

### Modifying Controller Logic

1. **Edit controller files** in `internal/controller/`
2. **Update reconcile functions**
3. **Add appropriate logging**:
   ```go
   log := r.Log.WithValues("sharedvolume", req.NamespacedName)
   log.Info("Processing SharedVolume", "newField", sharedVolume.Spec.NewField)
   ```
4. **Handle errors properly**:
   ```go
   if err != nil {
       log.Error(err, "Failed to process new field")
       return ctrl.Result{}, err
   }
   ```

### Adding Webhooks

1. **Create webhook implementation** in `internal/webhook/`:
   ```go
   func (w *SharedVolumeWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) error {
       // validation logic
   }
   ```

2. **Register webhook** in main.go:
   ```go
   if err := (&webhook.SharedVolumeWebhook{}).SetupWithManager(mgr); err != nil {
       setupLog.Error(err, "unable to create webhook", "webhook", "SharedVolume")
       os.Exit(1)
   }
   ```

3. **Update webhook configuration** in `config/webhook/`

## Testing Guidelines

### Unit Tests

```go
func TestSharedVolumeReconciler_Reconcile(t *testing.T) {
    tests := []struct {
        name     string
        volume   *svv1alpha1.SharedVolume
        expected ctrl.Result
        wantErr  bool
    }{
        {
            name: "successful reconcile",
            volume: &svv1alpha1.SharedVolume{
                // test data
            },
            expected: ctrl.Result{},
            wantErr:  false,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // test implementation
        })
    }
}
```

### Integration Tests

```go
func TestSharedVolumeIntegration(t *testing.T) {
    ctx := context.Background()
    
    // Create test SharedVolume
    volume := &svv1alpha1.SharedVolume{
        ObjectMeta: metav1.ObjectMeta{
            Name:      "test-volume",
            Namespace: "default",
        },
        Spec: svv1alpha1.SharedVolumeSpec{
            Capacity: "1Gi",
        },
    }
    
    err := k8sClient.Create(ctx, volume)
    assert.NoError(t, err)
    
    // Wait for reconciliation
    eventually := func() bool {
        err := k8sClient.Get(ctx, client.ObjectKeyFromObject(volume), volume)
        return err == nil && volume.Status.Phase == "Ready"
    }
    assert.Eventually(t, eventually, time.Minute, time.Second)
}
```

### End-to-End Tests

```bash
# E2E test script
#!/bin/bash
set -e

# Deploy controller
make deploy IMG=shared-volume-controller:e2e

# Create test resources
kubectl apply -f test/e2e/test-shared-volume.yaml

# Verify functionality
kubectl wait --for=condition=Ready sharedvolume/test-volume --timeout=300s

# Cleanup
kubectl delete -f test/e2e/test-shared-volume.yaml
make undeploy
```

## Debugging

### Enable Debug Logging

```bash
# Run with debug logging
make run ARGS="--zap-log-level=debug --zap-development=true"
```

### Using Delve Debugger

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug the controller
dlv debug ./cmd/main.go -- --kubeconfig ~/.kube/config
```

### Debugging in VS Code

Create `.vscode/launch.json`:

```json
{
    "version": "0.2.0",
    "configurations": [
        {
            "name": "Debug Controller",
            "type": "go",
            "request": "launch",
            "mode": "auto",
            "program": "${workspaceFolder}/cmd/main.go",
            "args": [
                "--kubeconfig", "~/.kube/config",
                "--zap-development=true"
            ],
            "env": {
                "KUBECONFIG": "~/.kube/config"
            }
        }
    ]
}
```

## Performance Optimization

### Profiling

```go
import _ "net/http/pprof"

// Add to main.go
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

Access profiler:
```bash
go tool pprof http://localhost:6060/debug/pprof/profile
```

### Metrics

Add custom metrics:

```go
import (
    "github.com/prometheus/client_golang/prometheus"
    "sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
    reconcileCounter = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "controller_reconcile_total",
            Help: "Total number of reconciliations",
        },
        []string{"controller", "result"},
    )
)

func init() {
    metrics.Registry.MustRegister(reconcileCounter)
}
```

## Release Process

### Creating a Release

1. **Update version** in relevant files
2. **Update CHANGELOG.md**
3. **Create and push tag**:
   ```bash
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0
   ```
4. **GitHub Actions will automatically**:
   - Build and test
   - Create Docker images
   - Generate release artifacts

### Versioning Strategy

- Follow [Semantic Versioning](https://semver.org/)
- Use pre-release versions for development: `v1.0.0-alpha.1`
- Tag releases: `v1.0.0`

## Code Style and Standards

### Go Code Style

- Follow [Effective Go](https://golang.org/doc/effective_go.html)
- Use `gofmt` and `goimports`
- Add meaningful comments for exported functions
- Use meaningful variable names

### Kubernetes Resources

- Follow Kubernetes API conventions
- Use proper resource naming
- Include appropriate labels and annotations
- Add RBAC permissions as needed

### Commit Messages

Follow conventional commits:
```
feat(controller): add support for custom storage classes
fix(webhook): handle nil pointer in pod validation
docs(readme): update installation instructions
```

## IDE Configuration

### VS Code

Recommended extensions:
- Go
- Kubernetes
- YAML
- GitLens

Settings (`.vscode/settings.json`):
```json
{
    "go.formatTool": "goimports",
    "go.lintTool": "golangci-lint",
    "go.testFlags": ["-v", "-race"],
    "yaml.schemas": {
        "kubernetes": "*.yaml"
    }
}
```

### GoLand/IntelliJ

- Enable Go modules support
- Configure code style to match project settings
- Set up run configurations for tests and main application

## Useful Make Targets

```bash
# Development
make run                 # Run controller locally
make test               # Run unit tests
make test-coverage      # Run tests with coverage
make fmt               # Format code
make vet               # Run go vet
make lint              # Run golangci-lint

# Building
make build             # Build binary
make docker-build     # Build Docker image
make docker-push      # Push Docker image

# Deployment
make install          # Install CRDs
make deploy           # Deploy to cluster
make undeploy         # Remove from cluster
make manifests        # Generate manifests
make generate         # Generate code

# Cleanup
make clean            # Clean build artifacts
```

## Getting Help

- Read the [API documentation](api.md)
- Check existing [issues](https://github.com/sharedvolume/shared-volume-controller/issues)
- Join [discussions](https://github.com/sharedvolume/shared-volume-controller/discussions)
- Review [examples](../examples/)

## Best Practices

1. **Always run tests** before submitting PRs
2. **Update documentation** for user-facing changes
3. **Add appropriate logging** for debugging
4. **Handle errors gracefully** with proper error messages
5. **Follow Kubernetes conventions** for resource management
6. **Write meaningful commit messages**
7. **Keep PRs focused** on single features or fixes
