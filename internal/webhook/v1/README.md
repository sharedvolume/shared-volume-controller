# Pod Mutating Webhook

This directory contains a mutating admission webhook for Kubernetes Pods that automatically adds labels based on pod annotations.

## Functionality

The webhook examines incoming Pod creation and update requests and:

1. **Checks for annotations**: Looks for any annotation keys that start with `sharedvolume.io/`
2. **Adds label**: If such annotations are found, adds the label `x: y` to the pod
3. **Continues processing**: Allows the pod creation/update to proceed

## Implementation Details

### Files

- `pod_webhook.go`: Main webhook implementation with the `PodAnnotator` struct
- `setup.go`: Webhook registration and setup functions
- `pod_webhook_test.go`: Unit tests for the webhook functionality

### Key Components

#### PodAnnotator

The `PodAnnotator` struct implements the `admission.Handler` interface and contains:

- `Client`: Kubernetes client for cluster access
- `decoder`: Admission decoder for parsing requests

#### Webhook Marker

The webhook uses kubebuilder markers to generate the webhook configuration:

```go
// +kubebuilder:webhook:path=/mutate-v1-pod,mutating=true,failurePolicy=fail,groups="",resources=pods,verbs=create;update,versions=v1,name=mpod.kb.io,admissionReviewVersions=v1
```

This generates the `MutatingAdmissionWebhook` configuration automatically.

## Usage Examples

### Pod with SharedVolume Annotations (will get x:y label)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: test-pod-with-sharedvolume
  annotations:
    sharedvolume.io/mount-path: "/mnt/shared"
    sharedvolume.io/sync-interval: "60s"
spec:
  containers:
  - name: test-container
    image: busybox:1.35
    command: ["sleep", "3600"]
```

**Result**: The webhook will add the label `x: y` to this pod.

### Pod without SharedVolume Annotations (no label added)

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: test-pod-regular
  annotations:
    app: "my-app"
    version: "1.0"
spec:
  containers:
  - name: test-container
    image: busybox:1.35
    command: ["sleep", "3600"]
```

**Result**: The webhook will not modify this pod.

## Testing

Run the unit tests:

```bash
go test ./internal/webhook/v1/...
```

The tests verify:
- Pods with `sharedvolume.io/` annotations receive the `x: y` label
- Pods without such annotations are not modified
- The webhook properly handles edge cases (no annotations, similar but non-matching annotations)

## Integration

The webhook is automatically registered when the controller starts if the `ENABLE_WEBHOOKS` environment variable is not set to `"false"`.

The webhook endpoint is exposed at `/mutate-v1-pod` and handles both CREATE and UPDATE operations for Pods.
