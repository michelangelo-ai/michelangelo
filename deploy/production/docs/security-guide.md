# Security Best Practices Guide

This guide provides comprehensive security recommendations for deploying Michelangelo in production environments.

## Security Framework

Michelangelo security follows a defense-in-depth approach with multiple layers:

```
┌─────────────────────────────────────────────────────────────┐
│                     Network Security                         │
├─────────────────────────────────────────────────────────────┤
│                  Application Security                        │
├─────────────────────────────────────────────────────────────┤
│                 Infrastructure Security                      │
├─────────────────────────────────────────────────────────────┤
│                      Data Security                          │
├─────────────────────────────────────────────────────────────┤
│                   Identity & Access                         │
└─────────────────────────────────────────────────────────────┘
```

## 🔐 Identity and Access Management

### Authentication

#### OIDC/OAuth2 Integration

```yaml
# API Server Configuration
apiserver:
  auth:
    enabled: true
    oidc:
      issuer: https://your-identity-provider.com
      clientId: michelangelo-api
      audience: michelangelo
      usernameClaim: email
      groupsClaim: groups
    jwt:
      signingKey: /etc/secrets/jwt-signing-key
      expirationHours: 24
```

#### Service Account Authentication

```yaml
# Use dedicated service accounts for each component
apiVersion: v1
kind: ServiceAccount
metadata:
  name: michelangelo-apiserver
  namespace: michelangelo-prod
  annotations:
    # AWS IAM role for service accounts
    eks.amazonaws.com/role-arn: arn:aws:iam::ACCOUNT:role/MichelangeloAPIServerRole
    # GCP Workload Identity
    iam.gke.io/gcp-service-account: michelangelo-api@PROJECT.iam.gserviceaccount.com
```

### Authorization

#### Role-Based Access Control (RBAC)

```yaml
# Define custom roles
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: michelangelo-prod
  name: michelangelo-pipeline-manager
rules:
- apiGroups: ["michelangelo.ai"]
  resources: ["pipelines", "pipelineruns"]
  verbs: ["get", "list", "create", "update", "patch"]
- apiGroups: [""]
  resources: ["secrets", "configmaps"]
  verbs: ["get", "list"]
  resourceNames: ["michelangelo-*"]  # Restrict to Michelangelo resources only

---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: michelangelo-developers
  namespace: michelangelo-prod
subjects:
- kind: Group
  name: michelangelo-developers
  apiGroup: rbac.authorization.k8s.io
roleRef:
  kind: Role
  name: michelangelo-pipeline-manager
  apiGroup: rbac.authorization.k8s.io
```

#### Fine-grained API Permissions

```yaml
# API Server RBAC configuration
apiserver:
  authorization:
    enabled: true
    policies:
      - name: pipeline-access
        rules:
          - subjects: ["group:data-scientists"]
            resources: ["pipelines:read", "models:read"]
          - subjects: ["group:ml-engineers"]
            resources: ["pipelines:*", "models:*", "deployments:*"]
          - subjects: ["group:admins"]
            resources: ["*:*"]
```

## 🛡️ Network Security

### Network Policies

```yaml
# Restrict inter-pod communication
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: michelangelo-network-policy
  namespace: michelangelo-prod
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/part-of: michelangelo-platform
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          name: michelangelo-prod
    - namespaceSelector:
        matchLabels:
          name: ingress-nginx
    ports:
    - protocol: TCP
      port: 14566  # API Server
    - protocol: TCP
      port: 8080   # Metrics
  egress:
  - to: []  # Allow all egress (customize based on requirements)
    ports:
    - protocol: TCP
      port: 443   # HTTPS
    - protocol: TCP
      port: 53    # DNS
    - protocol: UDP
      port: 53    # DNS
  - to:
    - namespaceSelector:
        matchLabels:
          name: temporal-system
    ports:
    - protocol: TCP
      port: 7233  # Temporal
```

### TLS Configuration

#### Enable TLS for All Components

```yaml
# API Server TLS
apiserver:
  tls:
    enabled: true
    certFile: /etc/tls/server.crt
    keyFile: /etc/tls/server.key
    caFile: /etc/tls/ca.crt
    clientAuth: require  # Mutual TLS

# Temporal TLS
workflow:
  temporal:
    tls:
      enabled: true
      certFile: /etc/temporal-tls/client.crt
      keyFile: /etc/temporal-tls/client.key
      serverName: temporal.yourdomain.com
```

#### Certificate Management with cert-manager

```yaml
# Certificate for API Server
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: michelangelo-api-tls
  namespace: michelangelo-prod
spec:
  secretName: michelangelo-api-tls
  issuer:
    name: letsencrypt-prod
    kind: ClusterIssuer
  dnsNames:
  - api.michelangelo.yourdomain.com
  usages:
  - digital signature
  - key encipherment
  - server auth
  - client auth  # For mutual TLS
```

### Ingress Security

```yaml
# Secure ingress configuration
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: michelangelo-api-ingress
  namespace: michelangelo-prod
  annotations:
    # Force HTTPS
    nginx.ingress.kubernetes.io/ssl-redirect: "true"
    nginx.ingress.kubernetes.io/force-ssl-redirect: "true"

    # Security headers
    nginx.ingress.kubernetes.io/server-snippet: |
      add_header X-Frame-Options "SAMEORIGIN" always;
      add_header X-Content-Type-Options "nosniff" always;
      add_header X-XSS-Protection "1; mode=block" always;
      add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;

    # Rate limiting
    nginx.ingress.kubernetes.io/rate-limit: "100"
    nginx.ingress.kubernetes.io/rate-limit-window: "1m"

    # IP whitelist (customize as needed)
    nginx.ingress.kubernetes.io/whitelist-source-range: "10.0.0.0/8,172.16.0.0/12,192.168.0.0/16"

    # Client certificate authentication
    nginx.ingress.kubernetes.io/auth-tls-verify-client: "optional"
    nginx.ingress.kubernetes.io/auth-tls-secret: "michelangelo-prod/client-ca"
spec:
  ingressClassName: nginx
  tls:
  - hosts:
    - api.michelangelo.yourdomain.com
    secretName: michelangelo-api-tls
  rules:
  - host: api.michelangelo.yourdomain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: michelangelo-apiserver
            port:
              number: 14566
```

## 🔒 Secrets Management

### External Secrets Integration

#### AWS Secrets Manager

```yaml
# External Secret for database credentials
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: michelangelo-database
  namespace: michelangelo-prod
spec:
  refreshInterval: 5m
  secretStoreRef:
    name: aws-secretsmanager
    kind: SecretStore
  target:
    name: michelangelo-database
    creationPolicy: Owner
  data:
  - secretKey: host
    remoteRef:
      key: michelangelo/prod/database
      property: host
  - secretKey: username
    remoteRef:
      key: michelangelo/prod/database
      property: username
  - secretKey: password
    remoteRef:
      key: michelangelo/prod/database
      property: password
```

#### HashiCorp Vault

```yaml
# Vault integration
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: vault-backend
  namespace: michelangelo-prod
spec:
  provider:
    vault:
      server: "https://vault.yourdomain.com"
      path: "secret"
      version: "v2"
      auth:
        kubernetes:
          mountPath: "kubernetes"
          role: "michelangelo"
          serviceAccountRef:
            name: "michelangelo-vault"
```

### Secret Rotation

```yaml
# Automated secret rotation using External Secrets
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: michelangelo-api-keys
spec:
  refreshInterval: 1h  # Check for updates every hour
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: michelangelo-api-keys
    creationPolicy: Owner
    template:
      type: Opaque
      metadata:
        annotations:
          reloader.stakater.com/match: "true"  # Auto-restart pods on secret change
```

## 🏗️ Infrastructure Security

### Pod Security Standards

```yaml
# Pod Security Policy (or Pod Security Standards)
apiVersion: v1
kind: Namespace
metadata:
  name: michelangelo-prod
  labels:
    pod-security.kubernetes.io/enforce: restricted
    pod-security.kubernetes.io/audit: restricted
    pod-security.kubernetes.io/warn: restricted
```

```yaml
# Security Context for all pods
apiVersion: apps/v1
kind: Deployment
metadata:
  name: michelangelo-apiserver
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65534
        runAsGroup: 65534
        fsGroup: 65534
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: apiserver
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65534
          capabilities:
            drop:
            - ALL
          seccompProfile:
            type: RuntimeDefault
```

### Resource Limits and Quotas

```yaml
# Resource quotas to prevent resource exhaustion attacks
apiVersion: v1
kind: ResourceQuota
metadata:
  name: michelangelo-quota
  namespace: michelangelo-prod
spec:
  hard:
    requests.cpu: "10"
    requests.memory: 20Gi
    limits.cpu: "20"
    limits.memory: 40Gi
    persistentvolumeclaims: "10"
    pods: "50"
    secrets: "20"
    configmaps: "20"

---
# Limit ranges for individual pods
apiVersion: v1
kind: LimitRange
metadata:
  name: michelangelo-limits
  namespace: michelangelo-prod
spec:
  limits:
  - default:
      memory: "1Gi"
      cpu: "500m"
    defaultRequest:
      memory: "256Mi"
      cpu: "100m"
    type: Container
```

### Image Security

```yaml
# Use specific image tags and signatures
apiVersion: apps/v1
kind: Deployment
metadata:
  name: michelangelo-apiserver
spec:
  template:
    spec:
      containers:
      - name: apiserver
        image: ghcr.io/michelangelo-ai/michelangelo/apiserver:v1.0.0@sha256:abcd1234...
        imagePullPolicy: Always
      imagePullSecrets:
      - name: ghcr-secret
```

#### Image Scanning Policy

```yaml
# OPA Gatekeeper policy for image scanning
apiVersion: templates.gatekeeper.sh/v1beta1
kind: ConstraintTemplate
metadata:
  name: allowedregistries
spec:
  crd:
    spec:
      names:
        kind: AllowedRegistries
      validation:
        properties:
          registries:
            type: array
            items:
              type: string
  targets:
    - target: admission.k8s.gatekeeper.sh
      rego: |
        package allowedregistries

        violation[{"msg": msg}] {
          container := input.review.object.spec.containers[_]
          not starts_with(container.image, input.parameters.registries[_])
          msg := sprintf("Container image %v is not from an allowed registry", [container.image])
        }

---
apiVersion: constraints.gatekeeper.sh/v1beta1
kind: AllowedRegistries
metadata:
  name: must-use-approved-registries
spec:
  match:
    kinds:
      - apiGroups: ["apps"]
        kinds: ["Deployment"]
    namespaces: ["michelangelo-prod"]
  parameters:
    registries:
      - "ghcr.io/michelangelo-ai/"
      - "gcr.io/your-project/"
```

## 🗃️ Data Security

### Encryption at Rest

#### Database Encryption

```yaml
# RDS with encryption
database:
  external:
    enabled: true
    host: michelangelo-prod-encrypted.region.rds.amazonaws.com
    sslMode: require
    encryption:
      enabled: true
      kmsKeyId: arn:aws:kms:region:account:key/key-id
```

#### Storage Encryption

```yaml
# Encrypted persistent volumes
apiVersion: v1
kind: StorageClass
metadata:
  name: encrypted-gp2
provisioner: kubernetes.io/aws-ebs
parameters:
  type: gp2
  encrypted: "true"
  kmsKeyId: arn:aws:kms:region:account:key/key-id
volumeBindingMode: WaitForFirstConsumer

---
# Use encrypted storage
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: michelangelo-data
spec:
  accessModes:
    - ReadWriteOnce
  storageClassName: encrypted-gp2
  resources:
    requests:
      storage: 10Gi
```

### Data Classification

```yaml
# Data classification labels
metadata:
  labels:
    data-classification: "confidential"
    data-retention: "7-years"
    data-location: "us-only"
  annotations:
    security.michelangelo.ai/pii: "true"
    security.michelangelo.ai/encryption-required: "true"
```

## 📊 Monitoring and Auditing

### Security Monitoring

```yaml
# Security-focused ServiceMonitor
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: michelangelo-security
  namespace: michelangelo-prod
spec:
  selector:
    matchLabels:
      app.kubernetes.io/part-of: michelangelo-platform
  endpoints:
  - port: metrics
    interval: 30s
    path: /metrics
    relabelings:
    - sourceLabels: [__name__]
      regex: '(auth_|security_|failed_|error_).*'
      targetLabel: __tmp_security_metric
      replacement: '${1}'
```

### Audit Logging

```yaml
# API Server audit configuration
apiserver:
  audit:
    enabled: true
    logPath: /var/log/michelangelo/audit.log
    logMaxAge: 30
    logMaxBackups: 10
    logMaxSize: 100
    policy:
      rules:
      - level: Metadata
        namespaces: ["michelangelo-prod"]
        resources:
        - group: "michelangelo.ai"
          resources: ["pipelines", "models", "deployments"]
        verbs: ["create", "update", "delete"]
      - level: Request
        users: ["system:serviceaccount:michelangelo-prod:admin"]
        verbs: ["create", "update", "delete"]
```

### Security Alerts

```yaml
# Prometheus alerting rules
groups:
- name: michelangelo-security
  rules:
  - alert: MichelangeloAuthenticationFailures
    expr: increase(michelangelo_auth_failures_total[5m]) > 10
    for: 0m
    labels:
      severity: warning
      component: authentication
    annotations:
      summary: "High authentication failure rate"
      description: "Michelangelo has {{ $value }} authentication failures in the last 5 minutes"

  - alert: MichelangeloUnauthorizedAccess
    expr: increase(michelangelo_unauthorized_requests_total[1m]) > 0
    for: 0m
    labels:
      severity: critical
      component: authorization
    annotations:
      summary: "Unauthorized access attempt detected"
      description: "Unauthorized access attempt detected in Michelangelo"

  - alert: MichelangeloPodSecurityViolation
    expr: increase(gatekeeper_violations_total{enforcement_action="deny"}[5m]) > 0
    for: 0m
    labels:
      severity: warning
      component: pod-security
    annotations:
      summary: "Pod security policy violation"
      description: "Pod security policy violation detected: {{ $labels.violation_kind }}"
```

## 🔍 Vulnerability Management

### Container Scanning

```yaml
# Trivy operator for continuous scanning
apiVersion: aquasecurity.github.io/v1alpha1
kind: ConfigAuditReport
metadata:
  name: deployment-michelangelo-apiserver
  namespace: michelangelo-prod
spec:
  artifact:
    kind: Deployment
    name: michelangelo-apiserver
    namespace: michelangelo-prod
  scanner:
    name: Trivy
    vendor: Aqua Security
    version: "0.35.0"
```

### Dependency Scanning

```bash
# CI/CD pipeline security scanning
jobs:
  security-scan:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v3

    - name: Run Trivy vulnerability scanner
      uses: aquasecurity/trivy-action@master
      with:
        image-ref: 'ghcr.io/michelangelo-ai/michelangelo/apiserver:latest'
        format: 'sarif'
        output: 'trivy-results.sarif'

    - name: Upload Trivy scan results to GitHub Security tab
      uses: github/codeql-action/upload-sarif@v2
      if: always()
      with:
        sarif_file: 'trivy-results.sarif'
```

## 🚨 Incident Response

### Security Incident Playbook

1. **Detection**: Monitor security alerts and logs
2. **Assessment**: Determine scope and impact
3. **Containment**: Isolate affected components
4. **Eradication**: Remove threats and vulnerabilities
5. **Recovery**: Restore normal operations
6. **Lessons Learned**: Update security measures

### Emergency Procedures

```bash
# Emergency pod isolation
kubectl label pod <pod-name> quarantine=true
kubectl patch networkpolicy michelangelo-network-policy -p '{"spec":{"podSelector":{"matchLabels":{"quarantine":"true"}}}}'

# Emergency secret rotation
kubectl delete secret michelangelo-api-keys
# External secrets will automatically recreate with new values

# Emergency access revocation
kubectl patch rolebinding michelangelo-developers -p '{"subjects":[]}'
```

## 📋 Security Checklist

### Pre-deployment Checklist

- [ ] Authentication configured (OIDC/JWT)
- [ ] RBAC policies defined and tested
- [ ] Network policies implemented
- [ ] TLS enabled for all components
- [ ] Secrets stored in external secret manager
- [ ] Image scanning enabled in CI/CD
- [ ] Pod security standards enforced
- [ ] Resource limits configured
- [ ] Audit logging enabled
- [ ] Monitoring and alerting configured

### Regular Security Reviews

- [ ] **Weekly**: Review access logs and security alerts
- [ ] **Monthly**: Update and rotate secrets
- [ ] **Quarterly**: Security vulnerability assessment
- [ ] **Annually**: Complete security audit and penetration testing

## 🔗 Additional Resources

- [OWASP Kubernetes Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Kubernetes_Security_Cheat_Sheet.html)
- [CIS Kubernetes Benchmark](https://www.cisecurity.org/benchmark/kubernetes)
- [NIST Cybersecurity Framework](https://www.nist.gov/cyberframework)
- [Kubernetes Security Best Practices](https://kubernetes.io/docs/concepts/security/)

For security issues or questions, contact the security team at security@michelangelo-ai.org.