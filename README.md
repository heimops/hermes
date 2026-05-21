# Hermes — Database Migration Operator

[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)
[![Helm Chart](https://img.shields.io/badge/helm-0.1.0-informational)](https://hermesops.github.io/hermes)
[![ArtifactHub](https://img.shields.io/endpoint?url=https://artifacthub.io/badge/repository/hermes)](https://artifacthub.io/packages/helm/hermes/hermes)
[![Kubernetes](https://img.shields.io/badge/kubernetes-1.25%2B-blue)](https://kubernetes.io)
[![OpenShift](https://img.shields.io/badge/openshift-4.12%2B-red)](https://www.redhat.com/en/technologies/cloud-computing/openshift)

Hermes is a Kubernetes operator that automates the migration of schema and data between relational databases. It supports MySQL and PostgreSQL (in any combination), and can run migrations once or keep databases in continuous sync on a cron schedule.

---

## Table of Contents

- [Overview](#overview)
- [Supported Databases](#supported-databases)
- [Architecture](#architecture)
- [Installation](#installation)
  - [Prerequisites](#prerequisites)
  - [Install with Helm](#install-with-helm)
  - [OpenShift](#openshift)
  - [Uninstall](#uninstall)
- [Quick Start](#quick-start)
  - [One-time Migration](#one-time-migration)
  - [Scheduled Synchronization](#scheduled-synchronization)
- [DatabaseMigration Reference](#databasemigration-reference)
  - [Spec Fields](#spec-fields)
  - [Status Fields](#status-fields)
- [Secret Management](#secret-management)
- [Helm Chart Configuration](#helm-chart-configuration)
- [How Migration Works](#how-migration-works)
  - [Schema Migration](#schema-migration)
  - [Data Migration](#data-migration)
  - [Type Conversion](#type-conversion)
- [Environment Variables](#environment-variables)
- [Upgrade](#upgrade)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [License](#license)

---

## Overview

Hermes introduces a `DatabaseMigration` Custom Resource (CRD) into your cluster. When you create one, the Hermes operator automatically:

1. Creates a Kubernetes `Job` (for one-time migrations) or a `CronJob` (for scheduled synchronization).
2. The Job/CronJob runs the **hermes-migrator** container, which connects to both databases, copies the schema (DDL), and copies the data in configurable batches.
3. Credentials are read from Kubernetes `Secret` objects — they are never stored in the CRD.

Hermes handles cross-database migrations transparently: if source and destination are different engines, it translates DDL and data types automatically.

---

## Supported Databases

| Database       | Version tested | Notes                         |
|----------------|---------------|-------------------------------|
| MySQL          | 5.7, 8.0      | Full support                  |
| MariaDB        | 10.5+         | Compatible with MySQL driver  |
| PostgreSQL     | 12, 14, 16    | Full support                  |

### Migration paths

| Source       | Destination  | Schema | Data |
|--------------|--------------|:------:|:----:|
| MySQL        | MySQL        | ✅     | ✅   |
| MySQL        | PostgreSQL   | ✅     | ✅   |
| PostgreSQL   | PostgreSQL   | ✅     | ✅   |
| PostgreSQL   | MySQL        | ✅     | ✅   |

---

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                          │
│                                                              │
│  ┌──────────────────┐       ┌──────────────────────────┐   │
│  │  Hermes Operator │       │  DatabaseMigration CR    │   │
│  │  (Deployment)    │◄─────►│  spec.mode: once/schedule│   │
│  └────────┬─────────┘       └──────────────────────────┘   │
│           │ creates                                          │
│           ▼                                                  │
│  ┌──────────────────┐       ┌──────────────────────────┐   │
│  │  Job / CronJob   │──────►│  hermes-migrator Pod     │   │
│  └──────────────────┘       │  ┌────────┐ ┌─────────┐  │   │
│                              │  │ Source │ │  Dest   │  │   │
│                              │  │   DB   │ │   DB    │  │   │
│                              │  └────────┘ └─────────┘  │   │
│                              └──────────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

**Components:**

| Component          | Language | Image                                     | Description                        |
|--------------------|----------|-------------------------------------------|------------------------------------|
| `hermes-operator`  | Go       | `ghcr.io/hermesops/hermes-operator:latest`| Watches CRDs, manages Jobs/CronJobs|
| `hermes-migrator`  | Go       | `ghcr.io/hermesops/hermes-migrator:latest`| Executes schema and data migration |

---

## Installation

### Prerequisites

- Kubernetes **1.25+** or OpenShift **4.12+**
- Helm **3.10+**
- `kubectl` configured to talk to your cluster
- (Optional) A container image registry if you are building custom images

### Install with Helm

Add the Hermes Helm repository:

```bash
helm repo add hermes https://hermesops.github.io/hermes
helm repo update
```

Install the operator into a dedicated namespace:

```bash
helm install hermes hermes/hermes \
  --namespace hermes-system \
  --create-namespace
```

Verify the operator is running:

```bash
kubectl get pods -n hermes-system
# NAME                               READY   STATUS    RESTARTS   AGE
# hermes-operator-6d8c9b7f5b-xkqlp  1/1     Running   0          30s

kubectl get crd databasemigrations.hermes.io
# NAME                              CREATED AT
# databasemigrations.hermes.io      2026-05-20T10:00:00Z
```

### OpenShift

On OpenShift 4.12+ the default `restricted-v2` Security Context Constraint is sufficient — both the operator and the migrator run as non-root users with a read-only root filesystem. No additional SCC configuration is required.

Install using the same Helm commands above. If your cluster has stricter network policies, ensure the migrator pods can reach the source and destination database endpoints.

```bash
# Optional: if your OpenShift cluster requires explicit SCC assignment
oc adm policy add-scc-to-user restricted-v2 \
  -z hermes-operator \
  -n hermes-system
```

### Uninstall

```bash
helm uninstall hermes -n hermes-system

# Remove the CRD (this also deletes all DatabaseMigration resources)
kubectl delete crd databasemigrations.hermes.io
```

---

## Quick Start

### One-time Migration

**Step 1 — Create Secrets with credentials:**

```bash
# Source database credentials
kubectl create secret generic source-db-secret \
  --namespace hermes-system \
  --from-literal=username=myuser \
  --from-literal=password=mysecretpassword

# Destination database credentials
kubectl create secret generic dest-db-secret \
  --namespace hermes-system \
  --from-literal=username=pguser \
  --from-literal=password=pgsecretpassword
```

**Step 2 — Create a `DatabaseMigration` resource:**

```yaml
# migration.yaml
apiVersion: hermes.io/v1alpha1
kind: DatabaseMigration
metadata:
  name: myapp-to-postgres
  namespace: hermes-system
spec:
  mode: once
  source:
    type: mysql
    host: mysql.database.svc.cluster.local
    port: 3306
    database: myapp
    secretRef:
      name: source-db-secret
  destination:
    type: postgresql
    host: postgres.database.svc.cluster.local
    port: 5432
    database: myapp
    secretRef:
      name: dest-db-secret
  options:
    schema: true
    data: true
    batchSize: 500
```

```bash
kubectl apply -f migration.yaml
```

**Step 3 — Check status:**

```bash
kubectl get dbm -n hermes-system
# NAME                 SOURCE                           DESTINATION                       MODE   READY   AGE
# myapp-to-postgres    mysql.database.svc.cluster.local postgres.database.svc.cluster.local once   True    2m

kubectl describe dbm myapp-to-postgres -n hermes-system

# Follow the migrator logs
kubectl logs -n hermes-system -l hermes/database-migration=myapp-to-postgres -f
```

---

### Scheduled Synchronization

Keep a replica database continuously in sync with the primary:

```yaml
# sync.yaml
apiVersion: hermes.io/v1alpha1
kind: DatabaseMigration
metadata:
  name: primary-to-replica
  namespace: hermes-system
spec:
  mode: schedule
  schedule: "0 */6 * * *"   # every 6 hours
  source:
    type: postgresql
    host: primary-pg.database.svc.cluster.local
    database: myapp
    secretRef:
      name: primary-db-secret
  destination:
    type: postgresql
    host: replica-pg.database.svc.cluster.local
    database: myapp
    secretRef:
      name: replica-db-secret
  options:
    schema: false        # schema already in sync
    data: true
    tables:              # only sync specific tables
      - orders
      - order_items
      - customers
    batchSize: 1000
```

```bash
kubectl apply -f sync.yaml

# Check the created CronJob
kubectl get cronjob -n hermes-system
# NAME                      SCHEDULE      SUSPEND   ACTIVE   LAST SCHEDULE   AGE
# hermes-primary-to-replica 0 */6 * * *   False     0        3h ago          1d
```

---

## DatabaseMigration Reference

### Spec Fields

#### Root

| Field         | Type             | Required | Default    | Description                                              |
|---------------|------------------|:--------:|------------|----------------------------------------------------------|
| `source`      | `DatabaseRef`    | ✅       | —          | Source database configuration                            |
| `destination` | `DatabaseRef`    | ✅       | —          | Destination database configuration                       |
| `mode`        | `string`         | —        | `once`     | `once` creates a Job; `schedule` creates a CronJob       |
| `schedule`    | `string`         | cron only| —          | Cron expression (e.g. `"0 2 * * *"`). Required if `mode=schedule` |
| `options`     | `MigrationOptions` | —      | —          | Controls what is migrated                                |
| `image`       | `string`         | —        | operator default | Override the migrator container image             |

#### DatabaseRef

| Field       | Type           | Required | Default | Description                                      |
|-------------|----------------|:--------:|---------|--------------------------------------------------|
| `type`      | `string`       | ✅       | —       | `mysql` or `postgresql`                          |
| `host`      | `string`       | ✅       | —       | Database server hostname or IP                   |
| `port`      | `integer`      | —        | 3306 / 5432 | Server port (defaults by type)              |
| `database`  | `string`       | ✅       | —       | Database / schema name                           |
| `secretRef` | `LocalSecretRef` | ✅     | —       | Reference to a Secret containing credentials     |
| `sslMode`   | `string`       | —        | —       | SSL mode: `disable`, `require`, `verify-full`, etc. |

#### LocalSecretRef

| Field         | Type     | Required | Default    | Description                         |
|---------------|----------|:--------:|------------|-------------------------------------|
| `name`        | `string` | ✅       | —          | Name of the Secret in the same namespace |
| `usernameKey` | `string` | —        | `username` | Key in the Secret for the username  |
| `passwordKey` | `string` | —        | `password` | Key in the Secret for the password  |

#### MigrationOptions

| Field       | Type       | Required | Default | Description                                              |
|-------------|------------|:--------:|---------|----------------------------------------------------------|
| `schema`    | `boolean`  | —        | `true`  | Migrate table schema (DDL: CREATE TABLE, etc.)           |
| `data`      | `boolean`  | —        | `true`  | Migrate row data                                         |
| `tables`    | `[]string` | —        | `[]`    | List of tables to migrate. Empty = all tables            |
| `batchSize` | `integer`  | —        | `1000`  | Number of rows read per batch during data migration      |

### Status Fields

| Field                | Type               | Description                                   |
|----------------------|--------------------|-----------------------------------------------|
| `jobRef`             | `string`           | Name of the created Job or CronJob            |
| `lastMigrationTime`  | `string` (RFC3339) | Time of the last successful migration         |
| `conditions`         | `[]Condition`      | Standard Kubernetes conditions                |

**Conditions:**

| Type    | Status | Reason           | Meaning                              |
|---------|--------|------------------|--------------------------------------|
| `Ready` | `True` | `JobSynced`      | Job created successfully (mode=once) |
| `Ready` | `True` | `CronJobSynced`  | CronJob synced (mode=schedule)       |
| `Ready` | `False`| `SyncFailed`     | Error creating or updating the job   |

---

## Secret Management

Credentials for the source and destination databases must be stored in Kubernetes Secrets in the **same namespace** as the `DatabaseMigration` resource.

**Minimum required keys:**

```bash
kubectl create secret generic my-db-secret \
  --from-literal=username=<db-user> \
  --from-literal=password=<db-password>
```

**Custom key names** (if your Secret uses different keys):

```yaml
secretRef:
  name: my-db-secret
  usernameKey: MYSQL_USER
  passwordKey: MYSQL_PASSWORD
```

**Recommended:** Use [External Secrets Operator](https://external-secrets.io/) or [Vault Agent Injector](https://developer.hashicorp.com/vault/docs/platform/k8s/injector) to populate Secrets from your secrets management system before creating the `DatabaseMigration`.

---

## Helm Chart Configuration

Full list of configurable values (`helm show values hermes/hermes`):

| Parameter                        | Default                                         | Description                              |
|----------------------------------|-------------------------------------------------|------------------------------------------|
| `operator.image.repository`      | `ghcr.io/hermesops/hermes-operator`             | Operator container image repository      |
| `operator.image.tag`             | `latest`                                        | Operator container image tag             |
| `operator.image.pullPolicy`      | `IfNotPresent`                                  | Image pull policy                        |
| `operator.replicas`              | `1`                                             | Number of operator replicas              |
| `operator.resources`             | `{}`                                            | CPU/memory resource requests and limits  |
| `migrator.image.repository`      | `ghcr.io/hermesops/hermes-migrator`             | Migrator container image repository      |
| `migrator.image.tag`             | `latest`                                        | Migrator container image tag             |
| `serviceAccount.create`          | `true`                                          | Create a ServiceAccount for the operator |
| `serviceAccount.name`            | `""`                                            | Override the ServiceAccount name         |
| `rbac.create`                    | `true`                                          | Create ClusterRole and ClusterRoleBinding|
| `leaderElection.enabled`         | `false`                                         | Enable leader election (for HA)          |

**Example — production installation with resource limits:**

```bash
helm install hermes hermes/hermes \
  --namespace hermes-system \
  --create-namespace \
  --set operator.replicas=2 \
  --set leaderElection.enabled=true \
  --set operator.resources.requests.cpu=100m \
  --set operator.resources.requests.memory=64Mi \
  --set operator.resources.limits.cpu=500m \
  --set operator.resources.limits.memory=128Mi
```

---

## How Migration Works

### Schema Migration

When `options.schema: true`, for each table the migrator:

1. Reads the table definition from the source database.
2. If source and destination are the **same engine**, the native `CREATE TABLE` DDL is replayed on the destination with `IF NOT EXISTS` added.
3. If they are **different engines**, the DDL is translated (column types are mapped, quoting conventions are adapted).
4. Executes the translated DDL on the destination.

### Data Migration

When `options.data: true`, for each table the migrator:

1. Truncates the destination table (logs a warning if the table doesn't exist yet, then continues).
2. Reads rows from the source in batches of `options.batchSize` rows (offset-based pagination).
3. Inserts each batch into the destination.
4. Repeats until all rows are copied.

> **Note:** The migrator does not currently maintain referential integrity order when migrating all tables. If your schema has foreign key constraints, either disable them temporarily on the destination or list tables in dependency order using `options.tables`.

### Type Conversion

When migrating between MySQL and PostgreSQL, Hermes applies the following type mapping:

**MySQL → PostgreSQL**

| MySQL type          | PostgreSQL type    |
|---------------------|--------------------|
| `TINYINT(1)`        | `BOOLEAN`          |
| `TINYINT`           | `SMALLINT`         |
| `SMALLINT`          | `SMALLINT`         |
| `MEDIUMINT`         | `INTEGER`          |
| `INT` / `INTEGER`   | `INTEGER`          |
| `BIGINT`            | `BIGINT`           |
| `FLOAT`             | `REAL`             |
| `DOUBLE`            | `DOUBLE PRECISION` |
| `DECIMAL(p,s)`      | `NUMERIC(p,s)`     |
| `VARCHAR(n)`        | `VARCHAR(n)`       |
| `CHAR(n)`           | `CHAR(n)`          |
| `TEXT` / `LONGTEXT` | `TEXT`             |
| `BLOB` / `LONGBLOB` | `BYTEA`            |
| `DATETIME`          | `TIMESTAMP`        |
| `DATE`              | `DATE`             |
| `TIME`              | `TIME`             |
| `YEAR`              | `INTEGER`          |
| `JSON`              | `JSONB`            |
| `ENUM` / `SET`      | `VARCHAR(255)`     |

**PostgreSQL → MySQL**

| PostgreSQL type       | MySQL type       |
|-----------------------|------------------|
| `BOOLEAN`             | `TINYINT(1)`     |
| `SMALLINT`            | `SMALLINT`       |
| `INTEGER`             | `INT`            |
| `BIGINT`              | `BIGINT`         |
| `REAL`                | `FLOAT`          |
| `DOUBLE PRECISION`    | `DOUBLE`         |
| `NUMERIC(p,s)`        | `DECIMAL(p,s)`   |
| `VARCHAR(n)`          | `VARCHAR(n)`     |
| `TEXT`                | `LONGTEXT`       |
| `BYTEA`               | `LONGBLOB`       |
| `TIMESTAMP`           | `DATETIME`       |
| `DATE`                | `DATE`           |
| `TIME`                | `TIME`           |
| `JSON` / `JSONB`      | `JSON`           |
| `UUID`                | `VARCHAR(36)`    |

---

## Environment Variables

The migrator container is configured entirely via environment variables (injected by the operator). These are useful when running the migrator standalone for testing:

| Variable                | Description                              |
|-------------------------|------------------------------------------|
| `HERMES_SOURCE_TYPE`    | `mysql` or `postgresql`                  |
| `HERMES_SOURCE_HOST`    | Source host                              |
| `HERMES_SOURCE_PORT`    | Source port                              |
| `HERMES_SOURCE_DATABASE`| Source database name                     |
| `HERMES_SOURCE_USERNAME`| Source username (from Secret)            |
| `HERMES_SOURCE_PASSWORD`| Source password (from Secret)            |
| `HERMES_SOURCE_SSL_MODE`| Source SSL mode                          |
| `HERMES_DEST_TYPE`      | `mysql` or `postgresql`                  |
| `HERMES_DEST_HOST`      | Destination host                         |
| `HERMES_DEST_PORT`      | Destination port                         |
| `HERMES_DEST_DATABASE`  | Destination database name                |
| `HERMES_DEST_USERNAME`  | Destination username (from Secret)       |
| `HERMES_DEST_PASSWORD`  | Destination password (from Secret)       |
| `HERMES_DEST_SSL_MODE`  | Destination SSL mode                     |
| `HERMES_MIGRATE_SCHEMA` | `true` / `false`                         |
| `HERMES_MIGRATE_DATA`   | `true` / `false`                         |
| `HERMES_TABLES`         | Comma-separated list of tables           |
| `HERMES_BATCH_SIZE`     | Rows per batch (default: 1000)           |

**Run the migrator standalone (for testing):**

```bash
docker run --rm \
  -e HERMES_SOURCE_TYPE=mysql \
  -e HERMES_SOURCE_HOST=localhost \
  -e HERMES_SOURCE_PORT=3306 \
  -e HERMES_SOURCE_DATABASE=myapp \
  -e HERMES_SOURCE_USERNAME=root \
  -e HERMES_SOURCE_PASSWORD=secret \
  -e HERMES_DEST_TYPE=postgresql \
  -e HERMES_DEST_HOST=localhost \
  -e HERMES_DEST_PORT=5432 \
  -e HERMES_DEST_DATABASE=myapp \
  -e HERMES_DEST_USERNAME=postgres \
  -e HERMES_DEST_PASSWORD=secret \
  ghcr.io/hermesops/hermes-migrator:latest
```

---

## Upgrade

```bash
helm repo update
helm upgrade hermes hermes/hermes -n hermes-system
```

> The CRD is included in the Helm chart. If the CRD schema changes between versions, Helm will update it automatically. Existing `DatabaseMigration` resources will continue to work.

---

## Troubleshooting

**The operator Pod is not starting:**
```bash
kubectl describe pod -n hermes-system -l app.kubernetes.io/name=hermes
kubectl logs -n hermes-system -l app.kubernetes.io/name=hermes
```

**The migration Job is not being created:**
```bash
kubectl describe dbm <name> -n <namespace>
# Check the "Conditions" section for error messages
```

**The migration Job failed:**
```bash
# Find the Job
kubectl get jobs -n <namespace> -l hermes/database-migration=<migration-name>

# Get the Pod logs
kubectl logs -n <namespace> -l hermes/database-migration=<migration-name>
```

**Common errors:**

| Error                          | Cause                                         | Fix                                       |
|--------------------------------|-----------------------------------------------|-------------------------------------------|
| `connect: connection refused`  | Database host is unreachable from the cluster | Check network policies and service names  |
| `Access denied for user`       | Wrong credentials in the Secret               | Verify the Secret keys and values         |
| `unknown database`             | Database does not exist on the destination    | Create the database before running Hermes |
| `foreign key constraint fails` | FK violation during truncate or insert        | Disable FK checks or list tables in order |

---

## Contributing

1. Fork the repository.
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes and add tests.
4. Run the test suite: `make test`
5. Submit a Pull Request.

Please read [CONTRIBUTING.md](CONTRIBUTING.md) for the full contribution guide.

---

## License

Apache License 2.0 — see [LICENSE](LICENSE) for details.
