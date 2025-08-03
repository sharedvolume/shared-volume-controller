# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial open source release
- MIT license adoption
- Comprehensive documentation and examples
- CI/CD workflows with GitHub Actions
- Security policy and vulnerability reporting process
- Container image publishing to Docker Hub

### Changed
- Updated license from Apache 2.0 to MIT
- Enhanced README with detailed usage instructions
- Improved project structure for open source development

### Security
- Added security scanning with Trivy
- Implemented security best practices documentation
- Added vulnerability reporting process

## [0.1.0] - 2025-01-XX

### Added
- SharedVolume CRD for namespace-scoped shared volumes
- ClusterSharedVolume CRD for cluster-scoped shared volumes
- NFS server integration for shared storage
- Admission webhooks for pod volume injection
- Data synchronization from Git, SSH, and S3 sources
- Basic controller functionality and reconciliation
- Pod cleanup controller for resource management

### Features
- Automatic volume mounting through pod annotations
- Support for multiple data sources
- Configurable sync policies and retry mechanisms
- Webhook-based validation and mutation
- Leader election for high availability

### Technical Details
- Built with Kubebuilder framework
- Go 1.24 support
- Kubernetes 1.19+ compatibility
- Distroless container images for security
