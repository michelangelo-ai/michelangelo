---
sidebar_position: 10
sidebar_label: "Compliance"
---

# Compliance Guide

Michelangelo supports compliance with SOC 2, GDPR, and HIPAA depending on how you deploy and configure the platform. Achieving compliance requires proper configuration of access controls, encryption, audit logging, and data handling practices described in this guide.

:::warning
Compliance is a shared responsibility. This guide covers Michelangelo-specific configuration. You are also responsible for ensuring your broader infrastructure, organizational processes, and legal agreements meet each framework's requirements.
:::

## SOC 2

SOC 2 (System and Organization Controls 2) evaluates controls across five Trust Service Criteria: security, availability, processing integrity, confidentiality, and privacy.

### Access Control

Enable RBAC for all Michelangelo resources and grant least-privilege access — users should only have the permissions required for their role.

```yaml
apiserver:
  auth:
    rbacEnabled: true
```

- Integrate with your organization's identity provider (IdP) via OIDC/OAuth 2.0
- Enforce multi-factor authentication (MFA) at the IdP level
- Set session token expiry appropriate for your security policy
- Disable any direct database or object store access that bypasses the Michelangelo API

### Encryption

- Enable TLS for all internal and external communication. Set `useTLS: true` in the Worker configuration.
- Use IAM roles for S3 access rather than hardcoded credentials (`useIam: true` in Controller Manager config)
- Enforce server-side encryption on your object store bucket (SSE-S3 or SSE-KMS)

### Network Security

- Deploy Michelangelo inside a private VPC with no direct public access to internal services
- Expose only the Envoy proxy and UI to controlled network segments
- Restrict Kubernetes API server access to authorized operators using network policies

### Availability

- Enable leader election for the Controller Manager (`leaderElection: true`) to prevent split-brain in HA deployments
- Run multiple replicas of the API Server and Controller Manager
- Configure liveness and readiness probes on all components
- Set up alerting on component health endpoints (`healthProbeBindAddress: 8081`)

### Audit Logging

All operations through the Michelangelo API are captured in audit logs. Ensure logs are:

- Forwarded to a centralized, tamper-resistant log store (e.g., CloudWatch, Splunk, Datadog)
- Retained for a minimum of 12 months
- Protected by access controls so only authorized personnel can read or delete them

### Confidentiality

- Restrict model artifacts, datasets, and experiment results using project-level RBAC
- Ensure object store bucket policies deny public access
- Rotate credentials and access tokens on a regular schedule

---

## GDPR

The General Data Protection Regulation (GDPR) applies when processing personal data of EU residents. Michelangelo can be configured to meet GDPR requirements around data residency, subject rights, and retention.

### Data Residency

Store all personal data in EU regions by configuring object storage to an EU endpoint:

```yaml
minio:
  awsRegion: eu-west-1
  awsEndpointUrl: s3.eu-west-1.amazonaws.com
  useIam: true
```

Ensure your Kubernetes cluster, Temporal/Cadence workflow engine, and any external data sources are also deployed in the same EU region.

### Data Minimization

- Only ingest and store data fields required for the ML task
- Use Michelangelo's data prep pipelines to strip personally identifiable information (PII) before training
- Avoid logging raw personal data in pipeline run outputs or model experiment metadata

### Data Subject Rights

**Right to Access**: Michelangelo's audit logs and model lineage tracking let you identify which data was used to train a specific model. Use these records to respond to Data Subject Access Requests (DSARs).

**Right to Erasure**: When a data subject requests deletion:
1. Identify all datasets, pipeline runs, and model versions that reference their data
2. Delete or anonymize the source data in your data store
3. Retrain or retire affected models where the data cannot be removed post-hoc
4. Document the erasure in your organization's records

**Right to Portability**: Use Michelangelo's export capabilities to produce data in standard formats (Parquet, CSV) when a subject requests their data.

### Retention Policies

- Define dataset and model artifact retention policies appropriate to your use case
- Periodically review and purge datasets containing personal data no longer needed for the original purpose
- Configure object store lifecycle rules to automatically expire old artifacts

### Consent and Legal Basis

Michelangelo does not manage consent records. Ensure your organization's consent management system is integrated with your data ingestion pipeline so that only data with a valid legal basis flows into Michelangelo training workflows.

### Data Processing Agreements

If you use cloud infrastructure providers (AWS, GCP, Azure) to host Michelangelo, you must have a Data Processing Agreement (DPA) in place with each provider before processing personal data on their infrastructure.

---

## HIPAA

The Health Insurance Portability and Accountability Act (HIPAA) applies when processing Protected Health Information (PHI). Compliance requires technical, administrative, and physical safeguards.

:::danger
Do not store PHI in Michelangelo unless your deployment has been reviewed by your compliance and legal teams and all safeguards below are confirmed to be in place. HIPAA violations carry significant legal and financial penalties.
:::

### Technical Safeguards

**Access Controls**

- Assign unique user IDs to every Michelangelo user — never use shared accounts
- Enable RBAC and restrict PHI-containing datasets and models to authorized users only
- Implement automatic session timeouts at the IdP level
- Audit all access to PHI-containing resources using Michelangelo's audit logs

**Encryption**

| Layer | Requirement | Configuration |
|-------|-------------|---------------|
| Data in transit | TLS 1.2+ | Set `useTLS: true` in Worker config; enforce HTTPS on Envoy |
| Data at rest | AES-256 | Enable SSE-KMS on your S3 bucket |
| Kubernetes secrets | Encrypted etcd | Enable etcd encryption-at-rest in your cluster configuration |

**Audit Controls**

HIPAA requires audit logs that record access to PHI. Configure your logging pipeline to capture:

- User identity and timestamp for every Michelangelo API call
- Dataset reads and writes
- Model training runs that reference PHI datasets
- Deployment and inference events

Forward logs to a HIPAA-eligible log management service and retain them for a minimum of **6 years**.

**Integrity Controls**

- Enable object versioning on your S3 bucket to detect unauthorized modification of artifacts
- Use checksums provided by Michelangelo's artifact storage to verify data integrity on read
- Configure S3 Object Lock (WORM mode) on audit log buckets to prevent tampering

**Transmission Security**

- Route all traffic through the Envoy proxy with TLS termination; disable any plaintext HTTP listeners
- Use VPN or private network peering for any cross-region traffic that may carry PHI

### Administrative Safeguards

- Designate a HIPAA Privacy Officer and Security Officer for your organization
- Conduct annual risk assessments that explicitly include your Michelangelo deployment
- Train all users with access to PHI-containing projects on HIPAA requirements and your organization's privacy policies
- Establish and enforce a workforce sanctions policy for unauthorized PHI access

### Business Associate Agreements (BAAs)

You must have a signed BAA with:

- Your cloud infrastructure provider (AWS, GCP, Azure)
- Any third-party services integrated with Michelangelo that may process PHI
- Your managed Temporal or Cadence service provider, if applicable

### PHI Readiness Checklist

Before processing PHI data in Michelangelo, verify each item:

- [ ] RBAC enabled and scoped to minimum necessary access
- [ ] TLS enabled on all components (`useTLS: true`)
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
- Apply Pod Security Standards (restricted profile) to Michelangelo workloads
- Regularly patch Kubernetes nodes and Michelangelo component images
- Store credentials in Kubernetes Secrets or an external secrets manager (HashiCorp Vault, AWS Secrets Manager) rather than in ConfigMaps

### Monitoring and Alerting

- Monitor the Controller Manager metrics endpoint (`metricsBindAddress: 8091`) for anomalies
- Alert on failed authentication attempts and privilege escalation events in your IdP logs
- Alert on unexpected data access patterns such as bulk dataset downloads outside of scheduled pipeline runs

### Incident Response

- Document an incident response runbook that covers Michelangelo-specific scenarios (unauthorized model access, artifact tampering, credential exposure)
- Test the runbook at least annually with tabletop exercises
- Ensure your runbook includes breach notification timelines:
  - **GDPR**: 72 hours to notify supervisory authority
  - **HIPAA**: 60 days to notify affected individuals and HHS
