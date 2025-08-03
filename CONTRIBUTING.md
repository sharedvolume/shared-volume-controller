# Contributing to Shared Volume Controller

Thank you for your interest in contributing to the Shared Volume Controller! This document provides guidelines and information for contributors.

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [How to Contribute](#how-to-contribute)
- [Development Setup](#development-setup)
- [Submitting Changes](#submitting-changes)
- [Style Guidelines](#style-guidelines)
- [Testing](#testing)
- [Documentation](#documentation)

## Code of Conduct

This project adheres to a code of conduct that we expect all contributors to follow. Please be respectful and constructive in all interactions.

## How to Contribute

### Reporting Bugs

1. **Check existing issues** - Look through existing issues to see if the bug has already been reported
2. **Create a detailed issue** - Include:
   - Clear description of the bug
   - Steps to reproduce
   - Expected vs actual behavior
   - Environment details (Kubernetes version, operator version, etc.)
   - Relevant logs or error messages

### Suggesting Features

1. **Search existing issues** to see if the feature has been requested
2. **Create a feature request** with:
   - Clear description of the feature
   - Use case and motivation
   - Possible implementation approach
   - Any alternatives considered

### Contributing Code

1. **Fork the repository**
2. **Create a feature branch** from `main`
3. **Make your changes**
4. **Add tests** for new functionality
5. **Ensure all tests pass**
6. **Submit a pull request**

## Development Setup

### Prerequisites

- Go 1.24 or later
- Docker
- kubectl
- kind (for local testing)
- make

### Setting Up Your Development Environment

1. **Clone your fork:**
   ```bash
   git clone https://github.com/YOUR_USERNAME/shared-volume-controller.git
   cd shared-volume-controller
   ```

2. **Install dependencies:**
   ```bash
   go mod download
   ```

3. **Set up a test cluster:**
   ```bash
   # Using kind
   kind create cluster --name shared-volume-dev
   
   # Or use your existing cluster
   kubectl config use-context your-cluster
   ```

4. **Install CRDs:**
   ```bash
   make install
   ```

5. **Run the controller locally:**
   ```bash
   make run
   ```

### Available Make Targets

- `make build` - Build the manager binary
- `make run` - Run against the configured Kubernetes cluster
- `make docker-build` - Build the docker image
- `make deploy` - Deploy to the K8s cluster
- `make undeploy` - Undeploy from the K8s cluster
- `make test` - Run unit tests
- `make fmt` - Run go fmt
- `make vet` - Run go vet
- `make lint` - Run golangci-lint

## Submitting Changes

### Pull Request Process

1. **Ensure your branch is up to date:**
   ```bash
   git fetch upstream
   git rebase upstream/main
   ```

2. **Run tests and linting:**
   ```bash
   make test
   make fmt
   make vet
   make lint
   ```

3. **Commit your changes:**
   - Use clear, descriptive commit messages
   - Follow the conventional commit format: `type(scope): description`
   - Examples:
     - `feat(controller): add support for custom storage classes`
     - `fix(webhook): handle nil pointer in pod validation`
     - `docs(readme): update installation instructions`

4. **Push your branch and create a pull request:**
   - Provide a clear title and description
   - Reference any related issues
   - Include screenshots or logs if relevant

### Pull Request Guidelines

- **One feature per PR** - Keep pull requests focused on a single feature or bug fix
- **Include tests** - All new functionality should have corresponding tests
- **Update documentation** - Update relevant documentation for user-facing changes
- **Follow existing patterns** - Maintain consistency with existing code style and architecture

## Style Guidelines

### Go Code Style

- Follow standard Go formatting (`gofmt`)
- Use meaningful variable and function names
- Add comments for exported functions and complex logic
- Follow the [Effective Go](https://golang.org/doc/effective_go.html) guidelines

### Kubernetes Resource Guidelines

- Follow Kubernetes API conventions
- Use proper resource naming and labeling
- Include appropriate RBAC permissions
- Add proper validation and defaulting

### Commit Message Format

We follow the [Conventional Commits](https://conventionalcommits.org/) specification:

```
<type>[optional scope]: <description>

[optional body]

[optional footer(s)]
```

Types:
- `feat`: A new feature
- `fix`: A bug fix
- `docs`: Documentation only changes
- `style`: Changes that do not affect the meaning of the code
- `refactor`: A code change that neither fixes a bug nor adds a feature
- `test`: Adding missing tests or correcting existing tests
- `chore`: Changes to the build process or auxiliary tools

## Testing

### Unit Tests

```bash
# Run all tests
make test

# Run specific package tests
go test ./internal/controller/...

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Integration Tests

```bash
# Run integration tests
make test-integration
```

### End-to-End Tests

```bash
# Run e2e tests
make test-e2e
```

### Test Guidelines

- Write tests for all new functionality
- Ensure tests are deterministic and can run in parallel
- Use table-driven tests where appropriate
- Mock external dependencies
- Include both positive and negative test cases

## Documentation

### Code Documentation

- Add godoc comments for all exported functions, types, and variables
- Include examples in documentation where helpful
- Keep comments up to date with code changes

### User Documentation

- Update the README.md for user-facing changes
- Add examples to the `examples/` directory
- Update API documentation for new CRD fields

### Documentation Guidelines

- Use clear, concise language
- Include code examples
- Explain the "why" not just the "how"
- Keep documentation up to date

## Release Process

Releases are handled by maintainers and follow semantic versioning:

1. Update version in relevant files
2. Create a release tag: `git tag -a v1.0.0 -m "Release v1.0.0"`
3. Push the tag: `git push origin v1.0.0`
4. GitHub Actions will automatically build and publish the release

## Getting Help

- **GitHub Discussions** - For questions and general discussion
- **GitHub Issues** - For bug reports and feature requests
- **Code Reviews** - Maintainers and community members will review your PRs

## Recognition

Contributors will be recognized in:
- The project's README
- Release notes for significant contributions
- GitHub's contributor graph

Thank you for contributing to Shared Volume Controller!
