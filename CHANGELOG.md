# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.7.0] - 2023-01-29

### Added

- Support for static bastion machines.

## [0.6.2] - 2022-10-07

### Fixed

- switched to a `podmonitor` for metrics scraping.

## [0.6.1] - 2022-10-06

### Fixed

- Immediately delete cache entries once a cluster got deleted.

## [0.6.0] - 2022-10-06

### Changed

- Remove CAPO go dependency.
- Normal reconciliation is only done if a cluster is in `Provisioned` state.
- Cache route53 API responses.
- Expose metrics about the internal cache.
- Remove the code piece that cleans old finalizers for migration.

## [0.5.0] - 2022-08-10

### Changed

- `dns-operator-openstack` now initially act on ClusterAPI `cluster` object to work with every
  infrastructure Provider via the `unstructured` client.

  Infrastructure specific information, like ClusterAPI OpenStack bastion IP can be queried via the
  raw `json path`
- `A`-Records for `bastion` hosts get cleaned up if no bastion host exists
- `dns-operator-openstack` is now build with `go 1.18`
- change `cluster-api-provider-openstack` packages from `v1alpha4` up to `v1alpha5` 
- Reduce requeue time from five minutes to one minute to react faster to nginx IC being installed.
- Improve finalizer addition&deletion to prevent unnecessary api calls.
- Add new parameter to make RBAC configurable for different infra providers.
- Renamed the project as `dns-operator-route53`.

## [0.4.1] - 2022-03-04

### Fixed

- Reduce normal reconciliation requeue time to 1 minute for faster change detection.

## [0.4.0] - 2022-02-22

### Changed

- Filter `nginx-ingress-controller` service by label and not name.

## [0.3.0] - 2022-02-01

### Changed

- Bump capo dependency to v0.5.0.

## [0.2.1] - 2022-01-20

### Fixed

- Fix bug with NS delegation.

## [0.2.0] - 2022-01-19

### Added

- Add MC metadata to hosted zones.
- Create route53 entries for bastion.

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

[Unreleased]: https://github.com/giantswarm/dns-operator-route53/compare/v0.7.0...HEAD
[0.7.0]: https://github.com/giantswarm/dns-operator-route53/compare/v0.6.2...v0.7.0
[0.6.2]: https://github.com/giantswarm/dns-operator-route53/compare/v0.6.1...v0.6.2
[0.6.1]: https://github.com/giantswarm/dns-operator-route53/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/giantswarm/dns-operator-route53/compare/v0.5.0...v0.6.0
[0.5.0]: https://github.com/giantswarm/dns-operator-route53/compare/v0.4.1...v0.5.0
[0.4.1]: https://github.com/giantswarm/dns-operator-openstack/compare/v0.4.0...v0.4.1
[0.4.0]: https://github.com/giantswarm/dns-operator-openstack/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/giantswarm/dns-operator-openstack/compare/v0.2.1...v0.3.0
[0.2.1]: https://github.com/giantswarm/dns-operator-openstack/compare/v0.2.0...v0.2.1
[0.2.0]: https://github.com/giantswarm/dns-operator-openstack/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/giantswarm/dns-operator-openstack/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/giantswarm/dns-operator-openstack/releases/tag/v0.1.0
