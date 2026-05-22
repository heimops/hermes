# Hermes — Database Migration Operator

Hermes is a Kubernetes operator that automates the migration of schema and data between relational databases. Define a `DatabaseMigration` resource and Hermes takes care of the rest: it creates a Kubernetes Job (one-time) or CronJob (scheduled) that connects to both databases, copies the structure and the data, and translates types automatically when source and destination use different engines.

## Supported databases

| Engine | Version |
|--------|---------|
| MySQL / MariaDB | 5.7, 8.0 / 10.5+ |
| PostgreSQL | 12, 14, 16 |

### Supported migration paths

| Source | Destination | Schema | Data |
|--------|-------------|:------:|:----:|
| MySQL | MySQL | ✅ | ✅ |
| MySQL | PostgreSQL | ✅ | ✅ |
| PostgreSQL | PostgreSQL | ✅ | ✅ |
| PostgreSQL | MySQL | ✅ | ✅ |

## Prerequisites

- Kubernetes **1.25+** or OpenShift **4.12+**
- Helm **3.10+**

## Installation

```bash
helm repo add hermes https://heimops.github.io/hermes
helm repo update
helm install hermes hermes/hermes \
  --namespace hermes-system \
  --create-namespace
```

Verify the operator is running:

```bash
kubectl get pods -n hermes-system
kubectl get crd databasemigrations.hermes.io
```

## Quick start

### 1 — Create Secrets with the database credentials

```bash
kubectl create secret generic source-db-secret \
  --namespace hermes-system \
  --from-literal=username=myuser \
  --from-literal=password=mypassword

kubectl create secret generic dest-db-secret \
  --namespace hermes-system \
  --from-literal=username=pguser \
  --from-literal=password=pgpassword
```

### 2 — One-time migration (MySQL → PostgreSQL)

```yaml
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

# Check status
kubectl get dbm -n hermes-system
kubectl logs -n hermes-system -l hermes/database-migration=myapp-to-postgres -f
```

### 3 — Scheduled synchronization (every 6 hours)

```yaml
apiVersion: hermes.io/v1alpha1
kind: DatabaseMigration
metadata:
  name: primary-to-replica
  namespace: hermes-system
spec:
  mode: schedule
  schedule: "0 */6 * * *"
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
    schema: false
    data: true
    tables:
      - orders
      - customers
    batchSize: 1000
```

## DatabaseMigration reference

### spec

| Field | Type | Required | Default | Description |
|-------|------|:--------:|---------|-------------|
| `source` | DatabaseRef | ✅ | — | Source database |
| `destination` | DatabaseRef | ✅ | — | Destination database |
| `mode` | string | — | `once` | `once` → Job · `schedule` → CronJob |
| `schedule` | string | if schedule | — | Cron expression, e.g. `"0 2 * * *"` |
| `options` | MigrationOptions | — | — | Controls what is migrated |
| `image` | string | — | operator default | Override the migrator image |

### spec.source / spec.destination (DatabaseRef)

| Field | Type | Required | Default | Description |
|-------|------|:--------:|---------|-------------|
| `type` | string | ✅ | — | `mysql` or `postgresql` |
| `host` | string | ✅ | — | Hostname or IP of the database server |
| `port` | integer | — | 3306 / 5432 | Server port |
| `database` | string | ✅ | — | Database name |
| `secretRef.name` | string | ✅ | — | Name of the Secret with credentials |
| `secretRef.usernameKey` | string | — | `username` | Key for the username inside the Secret |
| `secretRef.passwordKey` | string | — | `password` | Key for the password inside the Secret |
| `sslMode` | string | — | — | `disable`, `require`, `verify-full`, … |

### spec.options (MigrationOptions)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `schema` | boolean | `true` | Migrate table DDL (CREATE TABLE) |
| `data` | boolean | `true` | Migrate row data |
| `tables` | []string | `[]` | Tables to migrate — empty means **all** |
| `batchSize` | integer | `1000` | Rows read per batch |

### status

| Field | Description |
|-------|-------------|
| `jobRef` | Name of the created Job or CronJob |
| `lastMigrationTime` | Timestamp of the last successful run |
| `conditions[].type=Ready` | `True` when synced, `False` on error |

## Helm chart configuration

| Parameter | Default | Description |
|-----------|---------|-------------|
| `operator.image.repository` | `ghcr.io/heimops/hermes-operator` | Operator image |
| `operator.image.tag` | `latest` | Operator image tag |
| `operator.image.pullPolicy` | `IfNotPresent` | Pull policy |
| `operator.replicas` | `1` | Number of operator replicas |
| `operator.resources` | `{}` | Resource requests and limits |
| `migrator.image.repository` | `ghcr.io/heimops/hermes-migrator` | Migrator image used by Jobs |
| `migrator.image.tag` | `latest` | Migrator image tag |
| `serviceAccount.create` | `true` | Create a ServiceAccount |
| `serviceAccount.name` | `""` | Override ServiceAccount name |
| `rbac.create` | `true` | Create ClusterRole and ClusterRoleBinding |
| `leaderElection.enabled` | `false` | Enable leader election (HA deployments) |

### Production installation example

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

## OpenShift

The operator and the migrator both run as non-root users and are compatible with the default `restricted-v2` Security Context Constraint on OpenShift 4.12+. No additional SCC configuration is needed.

## Uninstall

```bash
helm uninstall hermes -n hermes-system
kubectl delete crd databasemigrations.hermes.io
```

> The CRD is installed with `helm.sh/resource-policy: keep` to protect existing `DatabaseMigration` resources. Delete it manually only when you are sure it is safe to do so.

## Troubleshooting

**Migration Job not created:**
```bash
kubectl describe dbm <name> -n <namespace>
# Check the Conditions section
```

**Migration failed:**
```bash
kubectl logs -n <namespace> -l hermes/database-migration=<name>
```

**Common errors:**

| Error | Cause | Fix |
|-------|-------|-----|
| `connection refused` | DB unreachable from the cluster | Check host, port and network policies |
| `Access denied` | Wrong credentials | Verify the Secret keys and values |
| `unknown database` | DB does not exist on destination | Create the database before running Hermes |
| `foreign key constraint fails` | FK violation during truncate | List tables in dependency order in `options.tables` |

## Source code and documentation

- GitHub: [https://github.com/heimops/hermes](https://github.com/heimops/hermes)
- Full documentation: [README](https://github.com/heimops/hermes/blob/main/README.md)
- Changelog: [CHANGELOG](https://github.com/heimops/hermes/blob/main/CHANGELOG.md)
- Issues: [https://github.com/heimops/hermes/issues](https://github.com/heimops/hermes/issues)
