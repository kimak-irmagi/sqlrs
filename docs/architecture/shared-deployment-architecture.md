# Shared Deployment Architecture (Team / Cloud)

Scope: how the `sqlrs` engine operates as a shared service in Team (A2) and Cloud (B3/C4) deployments. Focus on what changes from local: process model, ingress/auth, orchestration, storage, scaling, and isolation.

## 1. Goals

- Multi-tenant, authenticated access to the same engine logic (prepare planner/executor, cache, snapshotter).
- Horizontal scalability and high availability.
- Strong isolation between tenants (namespaces/policies/quotas).
- Shared state/cache and artifact storage with retention controls.
- Centralised observability and audit.

## 2. High-Level Topology

```mermaid
flowchart TD
  subgraph Edge
    GW[API Gateway]
    AUTH["Auth / OIDC / JWT"]
  end

  subgraph ControlPlane
    PROFILE["User Profile Service"]
    ORCH["Orchestrator (queue/prio/quotas)"]
    RUN["Runner Service (sqlrs engine instances)"]
    CACHE[state-cache service/index]
    ART[Artifact Store API]
    OBS[Telemetry/Audit]
    META[(Control metadata store)]
  end

  subgraph DataPlane
    ENV["env-manager (k8s executor)"]
    SNAP["snapshot store (PVC/S3)"]
    PG[(DB instances)]
  end

  Client --> GW --> AUTH --> ORCH
  GW --> PROFILE
  PROFILE --> META
  ORCH --> RUN
  RUN --> ENV
  RUN --> CACHE
  RUN --> ART
  RUN --> OBS
  ENV --> PG
  ENV --> SNAP
  CACHE --> SNAP
  CACHE --> META
  ORCH --> META
  GW --> OBS
```

## 3. Process and Request Flow

- Clients (CLI/IDE/UI) call Gateway with authenticated REST/gRPC for prepare jobs and cache/snapshot operations.
- Before authentication, CLI clients obtain installation-owned connection and
  provider metadata from public bootstrap routes at the root or any
  syntactically valid candidate organization prefix. These handlers do not
  query organization storage or disclose slug existence.
- Gateway enforces authN/authZ, rate limits, org quotas; forwards prepare and
  cache/snapshot operations to Orchestrator.
- Gateway forwards user and organization management requests to User Profile
  Service. The service creates and reads user profiles, external identity
  links, organizations, and memberships from server-owned user/org state.
- Orchestrator enqueues jobs with priority/quotas; dispatches to Runner instances.
- Runner (stateless engine) pulls job, performs prepare planning/cache lookup, asks env-manager to bind instance, executes prepare steps, snapshots, binds/selects instance, stores artefacts, updates status back to Orchestrator.
- Status/events streamed via Gateway (SSE/WS) for watch-mode clients.
- `run` commands execute locally via CLI against a prepared/shared instance; shared service does not run local commands.
- Script sources can be provided as a server-side project ref or as an uploaded `source_id` bundle.

## 4. Engine Changes vs Local

- **Lifecycle**: long-running service (Deployment) with HPA; no CLI-spawned processes.
- **Ingress**: behind Gateway; no loopback/UDS; auth required.
- **State store**: shared store (PVC/S3) + control metadata store or dedicated SQLite per shard synced to the control plane; per-tenant separation via namespaces/prefixes.
- **Cache service**: may run as separate service backing the engine’s cache client.
- **Liquibase**: runs as an external CLI inside controlled runner pods/containers; secrets from K8s Secrets/Vault. Overhead is measured and optimized if needed.
- **Snapshotter**: uses cluster storage (CSI snapshots/PVC + CoW if available); path resolution per tenant namespace.
- **Artifacts**: logs/reports exported to artifact store (S3/PVC) with retention tags.

## 5. Isolation and Security

- Auth: OIDC/JWT via Gateway; runner receives principal/org in token.
- Provider configuration is advertised only when Gateway trust accepts its
  exact issuer/client-ID pair. Inconsistent configuration makes bootstrap
  readiness fail and its routes return `503`.
- Gateway maps a validated issuer/client-ID pair to the stable advertised
  provider ID (for example `google`). User identity keys use that provider ID;
  protocol adapter names such as `oidc` are not identity-provider values.
- User Profile Service derives the current-user identity key for
  `PUT /v1/users/me` from validated OAuth/OIDC claims, can reject
  self-registration when disabled, and enforces external identity uniqueness
  over `provider + issuer + subject`.
- User Profile Service requires conditional writes for user profiles:
  `If-None-Match: *` for create-only registration/provisioning and
  `If-Match: <etag>` for update-only changes.
- Network: Namespace/NetworkPolicy to isolate instances; restrict egress.
- Storage: per-tenant prefixes in snapshot/artifact stores; ACLs enforced by service and backend IAM where applicable.
- Quotas/limits: enforced by Orchestrator and env-manager (CPU/RAM/TTL/concurrency).
- Secrets: managed by K8s Secrets or Vault/KMS; mounted/injected per job; never logged.

## 6. Scaling and Availability

- Runner service: HPA on queue backlog/latency metrics; multiple replicas; readiness/liveness probes.
- env-manager: scales instances; may use warm pools for fast start.
- Cache builders/GC: scale via autoscaling controller.
- Cluster autoscaler (Cloud): allowed with guard rails; Team may be ops-managed.

## 7. Persistence and Stores

- **State cache**: shared store with index; eviction policy respects org pins/retention.
- **Control metadata store**: metadata for jobs, states, artefacts, audit.
- **User profile store**: server-owned state behind User Profile Service.
  Storage technology is outside the current client slice; identity links are
  unique by provider, issuer, and subject.
- **Artifact store**: S3/PVC; immutable bundles for sharing.
- **Snapshot store**: CoW-friendly volumes or CSI snapshots; send/receive for remote copies when available.

## 8. Observability and Audit

- Metrics: queue length/age, runner latency, cache hit ratio, instance bind/start latency, snapshot sizes/time, errors.
- Logs: structured, centralised (Loki/ELK); correlated by job/prepare_id/org.
- Audit: prepare jobs, snapshots, sharing actions, scale events.
- User and organization lifecycle actions are audit events: self-registration,
  administrator-created users, organization creation, and membership changes.

## 9. Evolution Notes

- Same API contract as local `sqlrs` for prepare jobs (endpoint names TBD), but always async; watch via stream.
- Can shard cache/store by org or region; runner instances stateless except per-job instance.
- Future: pluggable executors beyond k8s; multi-region replication of cache/artifacts.

## 10. Service Component Structure (Jobs/Tasks)

### 10.1 Components and responsibilities

- **Gateway**
  - Exposes `GET /v1/prepare-jobs`, `DELETE /v1/prepare-jobs/{jobId}`,
    `GET /v1/tasks`, and the remote-only user/organization API.
  - Enforces authN/authZ and forwards to Orchestrator or User Profile Service.
- **User Profile Service**
  - Owns user profiles, external identity links, organizations, and memberships.
  - Applies self-registration policy and administrator authorization for manual
    user provisioning.
  - Implements `PUT /v1/users/me` and `PUT /v1/users/by-identity` as
    identity-keyed conditional writes.
  - Enforces unique external identity links.
- **Orchestrator**
  - Owns the job registry and task queue view.
  - Applies scheduling, quota, and deletion rules.
- **Runner**
  - Executes tasks and reports status transitions.
  - Streams logs/events for observability.
- **Control metadata store**
  - Persists jobs/tasks metadata and status history.

### 10.2 Key types and interfaces

- `PrepareJobEntry`, `TaskEntry`
  - List payloads for job/task queries.
- `UserProfile`, `ExternalIdentity`, `Organization`,
  `OrganizationMembership`
  - Payloads for user and organization management.
- `TaskStatus`
  - `queued | running | succeeded | failed`.
- `DeleteResult`
  - Common deletion outcome shape for job removal.

### 10.3 Data ownership

- The control metadata store is the source of truth for jobs/tasks in shared
  deployments.
- User Profile Service owns the source of truth for users, external identities,
  organizations, and memberships in shared deployments. The concrete storage
  technology is outside the current client slice.
- Orchestrator maintains in-memory queue state derived from the control
  metadata store for fast scheduling.
