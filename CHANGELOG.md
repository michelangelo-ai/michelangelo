# Changelog

All notable changes to the Michelangelo ML Platform will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

## [2025-08-20] - 2025-08-06 to 2025-08-20

### Added

- Enhanced table system with pagination, column controls, sorting, and drag-and-drop reordering
- Configuration-driven project views replacing hard-coded pipeline components
- Schema-based execution task rendering with recursive subtask navigation

### Changed

- Table components now persist state across browser sessions
- Project views use configurable PhaseEntityView system for extensibility
- Execution views include click-to-scroll navigation between overview and details

### Infrastructure

- New Cluster CRD and service for Kubernetes cluster lifecycle management
- Extension fields added to all API Spec and Status messages
- Automated Python SDK release pipeline with GitHub Actions

[Full release notes](./docs/releases/2025-08-06-to-2025-08-20.md)