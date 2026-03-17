# Drift Sentinel

Drift Sentinel is a Kubernetes validating admission controller that blocks unintended workload drift on `UPDATE` operations.

It compares the old and new object, strips Kubernetes-managed noise, applies rule-defined include and exclude scopes, and only allows changes that fall under explicitly mutable paths.

## Features

- full-object drift detection on `UPDATE`
- priority-based rule matching from annotated ConfigMaps
- namespace glob matching and API group/kind selectors
- include, exclude, and mutable path controls
- wildcard path matching for arrays and map keys
- mode handling: `enforce`, `warn`, `dry-run`, `off`
- resource-level bypass annotation
- namespace-level mode override
- live ConfigMap watch and rule cache reload via `client-go` informers
- structured JSON logs
- Prometheus metrics at `/metrics`
- Helm-managed webhook TLS secret generation

## Supported Annotations And Labels

ConfigMap rule discovery:

- `drift-sentinel.k8s.io/rule: "true"`

Resource bypass:

- default key: `drift-sentinel.k8s.io/bypass: "true"`
- this key is configurable per rule with the `bypass` field

Namespace mode override:

- `drift-sentinel.k8s.io/mode: "enforce|warn|dry-run|off"`

Namespace webhook opt-out label in the default chart:

- `drift-sentinel.k8s.io/enabled=false`

## How A Decision Is Made

For each `UPDATE` admission request:

1. Match the highest-priority rule by namespace glob and resource selector.
2. If no rule matches, allow the request.
3. If the resource has the rule bypass annotation set to `true`, allow the request.
4. Resolve a namespace mode override if present.
5. Strip implicit system fields from both objects:
   `status`, `metadata.managedFields`, `metadata.resourceVersion`, `metadata.generation`, `metadata.uid`, `metadata.creationTimestamp`, `metadata.selfLink`
6. Apply rule `include` paths if configured.
7. Apply rule `exclude` paths.
8. Diff the remaining object scope.
9. If there are no changes, allow the request.
10. Partition changed paths into `mutable` and immutable paths.
11. Enforce the effective mode.

## Rule ConfigMap Schema

Rules are stored in any namespace as annotated ConfigMaps:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: drift-sentinel-production
  namespace: drift-sentinel
  annotations:
    drift-sentinel.k8s.io/rule: "true"
data:
  spec: |
    mode: enforce
    priority: 200
    namespaces:
      - "prod-*"
    selectors:
      - apiGroup: "apps"
        kind: "Deployment"
      - apiGroup: "apps"
        kind: "StatefulSet"
    exclude:
      - "status"
      - "metadata.managedFields"
      - "metadata.resourceVersion"
    mutable:
      - "spec.template.spec.containers[*].image"
      - "spec.template.spec.initContainers[*].image"
    bypass: "drift-sentinel.k8s.io/bypass"
```

Supported rule fields:

- `mode`: `enforce`, `warn`, `dry-run`, `off`
- `priority`: higher wins; ties are broken deterministically by ConfigMap namespace/name
- `namespaces`: namespace glob patterns
- `selectors`: API group and kind pairs
- `exclude`: paths removed from comparison
- `include`: if set, only these paths are compared
- `mutable`: changed paths allowed within the compared scope
- `bypass`: resource annotation key that skips enforcement

## Supported Path Syntax

The path matcher supports a constrained JSONPath-like syntax:

- exact fields: `spec.replicas`
- array wildcard: `spec.template.spec.containers[*].image`
- exact array index in actual diff output: `spec.template.spec.containers[0].image`
- map wildcard: `metadata.*`
- quoted map key: `metadata.annotations['kubectl.kubernetes.io/last-applied-configuration']`

This is not full JSONPath. The implementation is intentionally narrow and deterministic.

## Mode Behavior

| Mode | Violation Behavior |
|------|--------------------|
| `enforce` | deny the request with `403` |
| `warn` | allow the request and log the violation |
| `dry-run` | allow the request and log a `would deny` reason |
| `off` | allow the request without blocking |

Precedence:

1. Resource bypass annotation
2. Namespace mode override annotation
3. Rule mode

## Default Webhook Scope

The Helm chart registers the webhook for:

- `apps/v1` `Deployment`
- `apps/v1` `StatefulSet`
- `apps/v1` `DaemonSet`
- `argoproj.io/v1alpha1` `Rollout`

These defaults are configurable in `values.yaml` under `webhook.rules`.

The engine itself matches on API group and kind from rules, but the webhook configuration controls which requests are actually sent to Drift Sentinel.

## Helm Chart

Chart path:

- `charts/drift-sentinel`

The chart renders:

- Deployment
- Service
- ServiceAccount
- ClusterRole and ClusterRoleBinding
- PodDisruptionBudget
- ValidatingWebhookConfiguration
- TLS Secret generated at render time
- default rule ConfigMap from `.Values.defaultRule.enabled`
- optional rule ConfigMaps from `.Values.rules`

Certificate behavior:

- the chart looks up `{{ fullname }}-cert` in the release namespace
- if it exists and contains `ca.crt`, `tls.crt`, and `tls.key`, it is reused
- otherwise Helm generates a CA and a serving certificate
- the same generated CA is used in the webhook `caBundle`

Default chart behavior:

- `webhook.failurePolicy` defaults to `Fail`
- `defaultRule.enabled` defaults to `true`
- the embedded default rule runs in `enforce` mode
- by default, drift is blocked for Deployments, StatefulSets, DaemonSets, and Rollouts except for container image changes

Basic install:

```bash
helm repo add drift-sentinel https://dheeth.github.io/drift-sentinel
helm install drift-sentinel drift-sentinel/drift-sentinel -n drift-sentinel --create-namespace
```

Install with embedded rules:

```yaml
rules:
  - name: drift-sentinel-production
    namespace: drift-sentinel
    spec: |
      mode: enforce
      priority: 200
      namespaces:
        - "prod-*"
      selectors:
        - apiGroup: "apps"
          kind: "Deployment"
      mutable:
        - "spec.template.spec.containers[*].image"
```

Example standalone rule manifests are available in:

- `deploy/examples/rule-strict.yaml`
- `deploy/examples/rule-relaxed.yaml`
- `deploy/examples/rule-replica-only.yaml`

## Local Development

Run against a local kubeconfig:

```powershell
$env:DRIFT_SENTINEL_KUBECONFIG="$HOME\.kube\config"
go run ./cmd/server
```

The service defaults to `:8080` locally. For webhook-style TLS serving, set:

- `DRIFT_SENTINEL_TLS_CERT_FILE`
- `DRIFT_SENTINEL_TLS_KEY_FILE`

## Runtime Configuration

| Variable | Default | Purpose |
|----------|---------|---------|
| `DRIFT_SENTINEL_ADDRESS` | `:8080` | HTTP or HTTPS listen address |
| `DRIFT_SENTINEL_LOG_LEVEL` | `INFO` | `slog` log level |
| `DRIFT_SENTINEL_HEALTH_PATH` | `/healthz` | health endpoint path |
| `DRIFT_SENTINEL_METRICS_PATH` | `/metrics` | metrics endpoint path |
| `DRIFT_SENTINEL_VALIDATE_PATH` | `/validate` | admission endpoint path |
| `DRIFT_SENTINEL_KUBECONFIG` | empty | use local kubeconfig instead of in-cluster config |
| `DRIFT_SENTINEL_TLS_CERT_FILE` | empty | TLS cert file path |
| `DRIFT_SENTINEL_TLS_KEY_FILE` | empty | TLS key file path |
| `DRIFT_SENTINEL_WATCH_RESYNC` | `30s` | informer resync period |
| `DRIFT_SENTINEL_STARTUP_SYNC_TIMEOUT` | `30s` | max time to wait for informer cache sync on startup |
| `DRIFT_SENTINEL_READ_HEADER_TIMEOUT` | `5s` | HTTP read header timeout |
| `DRIFT_SENTINEL_READ_TIMEOUT` | `15s` | HTTP read timeout |
| `DRIFT_SENTINEL_WRITE_TIMEOUT` | `15s` | HTTP write timeout |
| `DRIFT_SENTINEL_IDLE_TIMEOUT` | `60s` | HTTP idle timeout |
| `DRIFT_SENTINEL_SHUTDOWN_TIMEOUT` | `10s` | graceful shutdown timeout |

## Endpoints

- `GET /healthz`
- `GET /metrics`
- `POST /validate`

The admission endpoint expects `admission.k8s.io/v1` `AdmissionReview` requests and returns `AdmissionReview` responses.

## Metrics

The current metrics surface includes:

- `drift_sentinel_admission_requests_total{namespace,resource,result}`
- `drift_sentinel_violations_total{namespace,resource,field,mode}`
- `drift_sentinel_admission_duration_seconds`
- `drift_sentinel_rules_loaded_total`
- `drift_sentinel_config_events_total{event_type}`

## Logging

Admission decisions are logged as structured JSON, including:

- request UID
- operation
- namespace
- resource
- object name
- user
- matched rule
- effective mode
- result
- reason
- changed fields
- latency

## Current Limitations

- webhook scope is chart-defined; adding new resource types requires updating the webhook configuration
- rule parsing intentionally supports the documented schema rather than arbitrary YAML features
- certificate rotation is currently handled by Helm re-rendering or secret reuse, not by a controller loop in the app
- end-to-end cluster tests are not included yet

## Repository Guide

- `cmd/server`: process startup and HTTP server
- `pkg/admission`: AdmissionReview handling and validation decisions
- `pkg/diff`: path parsing, filtering, and deep diffing
- `pkg/rules`: rule parsing, matching, watching, and namespace mode resolution
- `charts/drift-sentinel`: Helm deployment
- `deploy/examples`: example rule manifests
