---
last_verified: 2026-10-14
sidebar_position: 2
sidebar_label: "Compliance"
---

# Compliance Guide

Michelangelo AI supports compliance with SOC 2, GDPR, and HIPAA depending on how you deploy and configure the platform. Achieving compliance requires proper configuration of access controls, encryption, audit logging, and data handling practices described in this guide.

**Audience**: Platform operators responsible for security and compliance configuration.

**Prerequisites**:
- Running Michelangelo AI control plane (see [Platform Setup](../setup/platform-setup.md))
- Kubernetes RBAC familiarity
- An identity provider (IdP) configured for OIDC/OAuth 2.0 (required for access control sections)
- Cloud provider account with S3-compatible object storage

:::warning
Compliance is a shared responsibility. This guide covers Michelangelo AI-specific configuration. You are also responsible for ensuring your broader infrastructure, organizational processes, and legal agreements meet each framework's requirements.
:::

## SOC 2

SOC 2 (System and Organization Controls 2) evaluates controls across five Trust Service Criteria: security, availability, processing integrity, confidentiality, and privacy.

### Access Control

The stock API server does not authenticate callers or enforce authorization by default (it wires `auth.DummyAuth`). Compliance requires you to enable authentication and authorization as described in [Authentication & Identity](../setup/authentication.md), and to grant least-privilege access with Kubernetes RBAC on the `michelangelo.api` resources — users should only have the permissions required for their role.

- Integrate with your organization's identity provider (IdP) via OIDC, using the `go/auth/oidc` token verifier wired into a custom API server build
- Enforce multi-factor authentication (MFA) at the IdP level
- Set token lifetimes at the IdP appropriate for your security policy
- Disable any direct database or object store access that bypasses the Michelangelo AI API

### Encryption

- Enable TLS for all internal and external communication. Use `apiserver.tls.enabled` for pod-level TLS on the API server, TLS on your Ingress, and `worker.useTLS: true` in the worker configuration when the worker reaches the API server over TLS (the chart does not render `useTLS`, so this needs a custom worker config)
- Keep object storage credentials in a Kubernetes Secret (`objectStorage.existingSecret`) rather than hardcoded in values, and scope them to the models bucket; set `objectStorage.secure: true`
- Enforce server-side encryption on your object store bucket (SSE-S3 or SSE-KMS)

### Network Security

- Deploy Michelangelo AI inside a private VPC with no direct public access to internal services
- Expose only the Envoy proxy and UI to controlled network segments
- Restrict Kubernetes API server access to authorized operators using network policies

### Availability

- Enable leader election for the Controller Manager (`controllermgr.leaderElection: true`, off by default) to prevent split-brain in HA deployments
- Run multiple replicas of the API Server and Controller Manager
- Configure liveness and readiness probes on all components
- Set up alerting on component health endpoints (`controllermgr.healthPort`, `8081` by default)

### Audit Logging

The API server emits audit events through the `logging.AuditLog` interface, whose default implementation is a no-op. To keep an audit trail you must provide your own `AuditLog` implementation, and also capture API server and Envoy access logs and Kubernetes audit logs. Ensure logs are:

- Forwarded to a centralized, tamper-resistant log store (e.g., CloudWatch, Splunk, Datadog)
- Retained for a minimum of 12 months
- Protected by access controls so only authorized personnel can read or delete them

### Confidentiality

- Restrict model artifacts, datasets, and experiment results using project-level RBAC
- Ensure object store bucket policies deny public access
- Rotate credentials and access tokens on a regular schedule

---

## GDPR

The General Data Protection Regulation (GDPR) applies when processing personal data of EU residents. Michelangelo AI can be configured to meet GDPR requirements around data residency, subject rights, and retention.

### Data Residency

Store all personal data in EU regions by configuring object storage to an EU endpoint:

```yaml
objectStorage:
  endpoint: s3.eu-west-1.amazonaws.com
  region: eu-west-1
  secure: true
  existingSecret: michelangelo-object-storage   # your own Secret
```

Ensure your Kubernetes cluster, Temporal/Cadence workflow engine, and any external data sources are also deployed in the same EU region.

### Data Minimization

- Only ingest and store data fields required for the ML task
- Strip personally identifiable information (PII) in the data preparation steps of your pipelines before training
- Avoid logging raw personal data in pipeline run outputs or model experiment metadata

### Data Subject Rights

**Right to Access**: Model resources reference the pipeline run that produced them (`spec.sourcePipelineRun`), and pipeline runs record their inputs, which lets you identify which data was used to train a specific model. Combine these records with your audit logs to respond to Data Subject Access Requests (DSARs).

**Right to Erasure**: When a data subject requests deletion:
1. Identify all datasets, pipeline runs, and model versions that reference their data
2. Delete or anonymize the source data in your data store
3. Retrain or retire affected models where the data cannot be removed post-hoc
4. Document the erasure in your organization's records

**Right to Portability**: Michelangelo AI does not store source datasets itself; export the subject's data from your own data stores in a standard format (for example Parquet or CSV) when requested.

### Retention Policies

- Define dataset and model artifact retention policies appropriate to your use case
- Periodically review and purge datasets containing personal data no longer needed for the original purpose
- Configure object store lifecycle rules to automatically expire old artifacts

### Consent and Legal Basis

Michelangelo AI does not manage consent records. Ensure your organization's consent management system is integrated with your data ingestion pipeline so that only data with a valid legal basis flows into Michelangelo AI training workflows.

### Data Processing Agreements

If you use cloud infrastructure providers (AWS, GCP, Azure) to host Michelangelo AI, you must have a Data Processing Agreement (DPA) in place with each provider before processing personal data on their infrastructure.

---

## HIPAA

The Health Insurance Portability and Accountability Act (HIPAA) applies when processing Protected Health Information (PHI). Compliance requires technical, administrative, and physical safeguards.

:::danger
Do not store PHI in Michelangelo AI unless your deployment has been reviewed by your compliance and legal teams and all safeguards below are confirmed to be in place. HIPAA violations carry significant legal and financial penalties.
:::

### Technical Safeguards

**Access Controls**

- Assign unique user IDs to every Michelangelo AI user — never use shared accounts
- Enable authentication and Kubernetes RBAC, and restrict PHI-containing datasets and models to authorized users only
- Implement automatic session timeouts at the IdP level
- Audit all access to PHI-containing resources using Michelangelo AI's audit logs

**Encryption**

| Layer | Requirement | Configuration |
|-------|-------------|---------------|
| Data in transit | TLS 1.2+ | Set `worker.useTLS: true` and `apiserver.tls.enabled` where applicable; enforce HTTPS on the Envoy and UI Ingress |
| Data at rest | AES-256 | Enable SSE-KMS on your S3 bucket |
| Kubernetes secrets | Encrypted etcd | Enable etcd encryption-at-rest in your cluster configuration |

**Audit Controls**

HIPAA requires audit logs that record access to PHI. Configure your logging pipeline to capture:

- User identity and timestamp for every Michelangelo AI API call (requires an authenticator and an `AuditLog` implementation, see above)
- Dataset reads and writes
- Model training runs that reference PHI datasets
- Deployment and inference events

Forward logs to a HIPAA-eligible log management service and retain them for a minimum of **6 years**.

**Integrity Controls**

- Enable object versioning on your S3 bucket to detect unauthorized modification of artifacts
- Use the checksums (for example S3 ETags or additional checksums) provided by your object store to verify data integrity on read
- Configure S3 Object Lock (WORM mode) on audit log buckets to prevent tampering

**Transmission Security**

- Terminate TLS for the Envoy proxy at your Ingress (`envoy.ingress.tls`); do not expose the plaintext Envoy or UI Services outside the cluster
- Use VPN or private network peering for any cross-region traffic that may carry PHI

### Administrative Safeguards

- Designate a HIPAA Privacy Officer and Security Officer for your organization
- Conduct annual risk assessments that explicitly include your Michelangelo AI deployment
- Train all users with access to PHI-containing projects on HIPAA requirements and your organization's privacy policies
- Establish and enforce a workforce sanctions policy for unauthorized PHI access

### Business Associate Agreements (BAAs)

You must have a signed BAA with:

- Your cloud infrastructure provider (AWS, GCP, Azure)
- Any third-party services integrated with Michelangelo AI that may process PHI
- Your managed Temporal or Cadence service provider, if applicable

### PHI Readiness Checklist

Before processing PHI data in Michelangelo AI, verify each item:

- [ ] Authentication enabled and RBAC scoped to minimum necessary access
- [ ] TLS enabled on all components (Ingress TLS, `apiserver.tls.enabled`, `worker.useTLS`)
- [ ] S3 bucket encryption (SSE-KMS) enabled
- [ ] S3 Object Lock (WORM) enabled on audit log buckets
- [ ] Audit logs forwarded to HIPAA-eligible log store with 6-year retention
- [ ] Automatic session timeout configured at IdP
- [ ] BAAs signed with all relevant vendors
- [ ] Annual risk assessment completed and documented
- [ ] Incident response plan covers PHI breach notification (60-day HIPAA deadline)

---

## General Security Recommendations

These practices improve your security posture across all three frameworks.

### Kubernetes Hardening

- Use Kubernetes Network Policies to restrict pod-to-pod communication to only required paths
- Apply Pod Security Standards (restricted profile) to Michelangelo AI workloads
- Regularly patch Kubernetes nodes and Michelangelo AI component images
- Store credentials in Kubernetes Secrets or an external secrets manager (HashiCorp Vault, AWS Secrets Manager) rather than in ConfigMaps (note that the `michelangelo-config` ConfigMap in compute namespaces carries storage credentials in plaintext; scope who can read it)

### Monitoring and Alerting

- Monitor the Controller Manager metrics endpoint (`controllermgr.metricsPort`, `8091` by default) for anomalies
- Alert on failed authentication attempts and privilege escalation events in your IdP logs
- Alert on unexpected data access patterns such as bulk dataset downloads outside of scheduled pipeline runs

### Incident Response

- Document an incident response runbook that covers Michelangelo AI-specific scenarios (unauthorized model access, artifact tampering, credential exposure)
- Test the runbook at least annually with tabletop exercises
- Ensure your runbook includes breach notification timelines:
  - **GDPR**: 72 hours to notify supervisory authority
  - **HIPAA**: 60 days to notify affected individuals and HHS
