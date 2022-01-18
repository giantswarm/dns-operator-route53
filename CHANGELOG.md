# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Add MC metadata to hosted zones.

### Changed

- Read the cluster hosted zone only once per operation.

### Fixed

- Do not fail on already deleted entries.
- Remove `alreadyExistsError` on creation/update as it's obsolete with UPSERT.

## [0.1.1] - 2022-01-05

### Added

- Add core Cluster to scope in addition to infrastructure cluster.

### Changed

- Rename scope OpenStackCluster to InfrastructureCluster for consistency.

### Fixed

- Look up WC kubeconfig based on Cluster name instead of OpenStackCluster name.
- Update changed DNS record values.
- Use name from Cluster instead of OpenStackCluster.
- Skip WC ingress IP lookup during cluster deletion.
- Cache WC k8s client in scope.
- Create WC k8s client when needed rather than on every reconciliation loop.

## [0.1.0] - 2021-12-15

### Added

- Create api and ingress entries in Route53.

[Unreleased]: https://github.com/giantswarm/dns-operator-openstack/compare/v0.1.1...HEAD
[0.1.1]: https://github.com/giantswarm/dns-operator-openstack/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/giantswarm/dns-operator-openstack/releases/tag/v0.1.0
