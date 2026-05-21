# Changelog

All notable changes to the Hermes Database Migration Operator are documented in this file.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

---

## [0.1.0] — 2026-05-20

### Added

#### Operator
- `DatabaseMigration` Custom Resource Definition (`hermes.io/v1alpha1`).
- Short name `dbm` for `kubectl get dbm`.
- Reconciler that creates a Kubernetes `batch/v1 Job` when `spec.mode: once`.
- Reconciler that creates a Kubernetes `batch/v1 CronJob` when `spec.mode: schedule`.
- Owner references on managed Jobs and CronJobs — resources are automatically garbage-collected when the parent `DatabaseMigration` is deleted.
- `Ready` status condition updated after each reconcile cycle.
- `status.jobRef` field tracking the name of the managed Job or CronJob.
- `status.lastMigrationTime` field updated from the Job/CronJob completion timestamp.
- Leader election support (`--leader-elect` flag) for high-availability deployments.
- Health (`/healthz`) and readiness (`/readyz`) probe endpoints.
- Prometheus-compatible metrics endpoint on `:8080`.
- `--default-image` flag to configure the default migrator container image.

#### Migrator
- MySQL source support via `github.com/go-sql-driver/mysql`.
- PostgreSQL source support via `github.com/lib/pq`.
- MySQL destination support.
- PostgreSQL destination support.
- Offset-based batched data migration (configurable batch size, default 1000 rows).
- Schema migration: reads DDL from source, applies to destination with `IF NOT EXISTS` guard.
- Cross-database DDL translation: MySQL → PostgreSQL and PostgreSQL → MySQL.
- Type mapping for 20+ common column types in each direction.
- Structured JSON logging via `log/slog`.
- All configuration via `HERMES_*` environment variables (injected by the operator from the CRD spec and referenced Secrets).

#### Helm Chart (`hermes/hermes`)
- CRD template with `helm.sh/resource-policy: keep` annotation to prevent accidental deletion on `helm uninstall`.
- Operator `Deployment` with configurable replicas and resource limits.
- `ClusterRole` and `ClusterRoleBinding` with least-privilege RBAC (read/write `databasemigrations`, `jobs`, `cronjobs`).
- `ServiceAccount` for the operator Pod.
- Configurable operator and migrator image repositories and tags.

#### Configuration Samples
- `config/samples/databasemigration_mysql_to_postgresql.yaml` — one-time migration from MySQL to PostgreSQL.
- `config/samples/databasemigration_postgresql_schedule.yaml` — scheduled sync between two PostgreSQL instances.

#### Infrastructure
- Multi-stage Dockerfiles for both `hermes-operator` and `hermes-migrator` using `gcr.io/distroless/static:nonroot` as runtime base.
- Root `Makefile` with `build`, `test`, `lint`, and `docker-build` targets.
- ArtifactHub repository metadata (`artifacthub-repo.yml`).
- GitHub Actions workflow for automated Helm chart release to GitHub Pages.

---

[Unreleased]: https://github.com/hermesops/hermes/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/hermesops/hermes/releases/tag/v0.1.0
