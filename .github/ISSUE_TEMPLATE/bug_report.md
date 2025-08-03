---
name: Bug Report
about: Create a report to help us improve
title: '[BUG] '
labels: 'bug'
assignees: ''

---

**Describe the bug**
A clear and concise description of what the bug is.

**To Reproduce**
Steps to reproduce the behavior:
1. Create SharedVolume with '...'
2. Apply pod with '...'
3. Check status '...'
4. See error

**Expected behavior**
A clear and concise description of what you expected to happen.

**Actual behavior**
A clear and concise description of what actually happened.

**Environment**
- Kubernetes version: [e.g., v1.28.0]
- Shared Volume Controller version: [e.g., v1.0.0]
- Container runtime: [e.g., containerd, docker]
- Cloud provider: [e.g., AWS, GCP, Azure, on-premise]
- Storage class: [e.g., gp2, standard]

**Resource Definitions**
```yaml
# Please include the YAML definitions of relevant resources
apiVersion: sv.sharedvolume.io/v1alpha1
kind: SharedVolume
metadata:
  name: example
spec:
  # your spec here
```

**Logs**
```
# Controller logs
kubectl logs -n shared-volume-controller-system deployment/shared-volume-controller-manager

# Pod logs (if applicable)
kubectl logs <pod-name> -n <namespace>
```

**Additional context**
Add any other context about the problem here, such as:
- Network policies in use
- Security contexts
- Custom RBAC configurations
- Recent changes to the cluster

**Screenshots**
If applicable, add screenshots to help explain your problem.
