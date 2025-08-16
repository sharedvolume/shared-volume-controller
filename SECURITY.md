# Security Policy

## Supported Versions

We support security updates for the following versions:

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| 0.x.x   | :x: (Development)  |

## Reporting a Vulnerability

We take security seriously. If you discover a security vulnerability, please follow these steps:

### 1. Do NOT create a public issue

Please do not report security vulnerabilities through public GitHub issues.

### 2. Send a private report

Email security details to: **security@sharedvolume.io**

Include the following information:
- Description of the vulnerability
- Steps to reproduce the issue
- Potential impact
- Any suggested fixes (if you have them)

### 3. Response Timeline

- **Initial Response**: Within 48 hours
- **Investigation**: 1-7 days depending on complexity
- **Fix Development**: 1-14 days depending on severity
- **Public Disclosure**: After fix is released and users have time to update

### 4. Security Update Process

1. We will confirm the vulnerability
2. Develop and test a fix
3. Create a security release
4. Notify users through:
   - GitHub Security Advisories
   - Release notes
   - Email (for critical vulnerabilities)

## Security Best Practices

### For Users

1. **Keep Updated**: Always use the latest version
2. **RBAC**: Follow principle of least privilege
3. **Network Policies**: Restrict network access where possible
4. **Secrets Management**: Use Kubernetes secrets for sensitive data
5. **Image Security**: Scan container images for vulnerabilities

### For Contributors

1. **Dependency Updates**: Keep dependencies updated
2. **Code Review**: All changes require review
3. **Testing**: Include security-focused tests
4. **Static Analysis**: Use security linting tools
5. **Secret Scanning**: Never commit secrets

## Known Security Considerations

### Webhooks

- Webhook certificates should be properly managed
- Validate all webhook requests
- Use TLS for all webhook communications

### NFS Security

- NFS connections may be unencrypted
- Consider network policies for NFS traffic
- Validate NFS server configurations

### Pod Security

- Pods with shared volumes may have elevated privileges
- Consider Pod Security Standards
- Validate volume mount permissions

## Responsible Disclosure

We appreciate responsible disclosure of security vulnerabilities. We commit to:

1. Acknowledging your contribution
2. Keeping you informed of our progress
3. Crediting you in the security advisory (unless you prefer anonymity)

Thank you for helping keep Shared Volume Controller secure!
