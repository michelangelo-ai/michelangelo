# AWS Deployment Guide

This guide provides step-by-step instructions for deploying Michelangelo on AWS using Amazon EKS, RDS, and S3.

## Prerequisites

Before starting, ensure you have:

- **AWS CLI** configured with appropriate permissions
- **kubectl** installed and configured
- **helm** (v3.0+) if using Helm deployment
- **eksctl** for EKS cluster management
- **terraform** (optional, for Infrastructure as Code)

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                        AWS Cloud                            │
├─────────────────────────────────────────────────────────────┤
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │     EKS     │  │     RDS     │  │        S3           │  │
│  │   Cluster   │  │   (MySQL)   │  │   (Artifacts)       │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
│         │                 │                    │            │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────────────┐  │
│  │   ALB/NLB   │  │ Temporal/   │  │      IAM Roles      │  │
│  │  (Ingress)  │  │  Cadence    │  │   (Service Accts)   │  │
│  └─────────────┘  └─────────────┘  └─────────────────────┘  │
└─────────────────────────────────────────────────────────────┘
```

## Step 1: Infrastructure Setup

### Option A: Using eksctl (Recommended for quick setup)

```bash
# Create EKS cluster
eksctl create cluster \
  --name michelangelo-prod \
  --version 1.24 \
  --region us-west-2 \
  --nodegroup-name standard-workers \
  --node-type m5.large \
  --nodes 3 \
  --nodes-min 2 \
  --nodes-max 10 \
  --managed \
  --with-oidc \
  --ssh-access \
  --ssh-public-key ~/.ssh/id_rsa.pub

# Enable OIDC provider for IRSA (IAM Roles for Service Accounts)
eksctl utils associate-iam-oidc-provider \
  --region=us-west-2 \
  --cluster=michelangelo-prod \
  --approve
```

### Option B: Using Terraform

```bash
# Clone Terraform configurations
cd deploy/production/terraform/aws

# Initialize Terraform
terraform init

# Review and customize variables
cp terraform.tfvars.example terraform.tfvars
vim terraform.tfvars

# Plan and apply
terraform plan
terraform apply
```

## Step 2: Database Setup (RDS)

### Create RDS MySQL Instance

```bash
# Using AWS CLI
aws rds create-db-instance \
  --db-instance-identifier michelangelo-prod-db \
  --db-instance-class db.t3.medium \
  --engine mysql \
  --engine-version 8.0.32 \
  --master-username michprod \
  --master-user-password 'YourSecurePassword123!' \
  --allocated-storage 100 \
  --storage-type gp2 \
  --storage-encrypted \
  --vpc-security-group-ids sg-xxxxxxxxxx \
  --db-subnet-group-name michelangelo-db-subnet-group \
  --backup-retention-period 7 \
  --multi-az \
  --deletion-protection
```

### Configure Database Access

```bash
# Create database and user
mysql -h michelangelo-prod-db.region.rds.amazonaws.com -u michprod -p <<EOF
CREATE DATABASE michelangelo_prod;
CREATE USER 'michelangelo'@'%' IDENTIFIED BY 'SecureAppPassword123!';
GRANT ALL PRIVILEGES ON michelangelo_prod.* TO 'michelangelo'@'%';
FLUSH PRIVILEGES;
EOF

# Store credentials in AWS Secrets Manager
aws secretsmanager create-secret \
  --name "michelangelo/prod/database" \
  --description "Michelangelo production database credentials" \
  --secret-string '{
    "host": "michelangelo-prod-db.region.rds.amazonaws.com",
    "port": "3306",
    "database": "michelangelo_prod",
    "username": "michelangelo",
    "password": "SecureAppPassword123!"
  }'
```

## Step 3: S3 Storage Setup

```bash
# Create S3 bucket for artifacts
aws s3 mb s3://michelangelo-artifacts-prod-${RANDOM}

# Enable versioning and encryption
aws s3api put-bucket-versioning \
  --bucket michelangelo-artifacts-prod-${RANDOM} \
  --versioning-configuration Status=Enabled

aws s3api put-bucket-encryption \
  --bucket michelangelo-artifacts-prod-${RANDOM} \
  --server-side-encryption-configuration '{
    "Rules": [{
      "ApplyServerSideEncryptionByDefault": {
        "SSEAlgorithm": "AES256"
      }
    }]
  }'

# Create IAM policy for S3 access
cat > michelangelo-s3-policy.json <<EOF
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:GetObject",
        "s3:PutObject",
        "s3:DeleteObject",
        "s3:ListBucket"
      ],
      "Resource": [
        "arn:aws:s3:::michelangelo-artifacts-prod-*",
        "arn:aws:s3:::michelangelo-artifacts-prod-*/*"
      ]
    }
  ]
}
EOF

aws iam create-policy \
  --policy-name MichelangeloS3Access \
  --policy-document file://michelangelo-s3-policy.json
```

## Step 4: IAM Roles for Service Accounts (IRSA)

```bash
# Create IAM role for Michelangelo components
eksctl create iamserviceaccount \
  --name michelangelo-worker \
  --namespace michelangelo-prod \
  --cluster michelangelo-prod \
  --attach-policy-arn arn:aws:iam::ACCOUNT:policy/MichelangeloS3Access \
  --approve

# Create IAM role for AWS Load Balancer Controller
eksctl create iamserviceaccount \
  --cluster=michelangelo-prod \
  --namespace=kube-system \
  --name=aws-load-balancer-controller \
  --attach-policy-arn=arn:aws:iam::AWS_ACCOUNT_ID:policy/AWSLoadBalancerControllerIAMPolicy \
  --override-existing-serviceaccounts \
  --approve
```

## Step 5: Install Required Controllers

### AWS Load Balancer Controller

```bash
# Add helm repo
helm repo add eks https://aws.github.io/eks-charts
helm repo update

# Install AWS Load Balancer Controller
helm install aws-load-balancer-controller eks/aws-load-balancer-controller \
  -n kube-system \
  --set clusterName=michelangelo-prod \
  --set serviceAccount.create=false \
  --set serviceAccount.name=aws-load-balancer-controller
```

### cert-manager (for TLS)

```bash
# Install cert-manager
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.11.0/cert-manager.yaml

# Create ClusterIssuer for Let's Encrypt
cat <<EOF | kubectl apply -f -
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: admin@yourdomain.com
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
    - http01:
        ingress:
          class: alb
EOF
```

### External Secrets Operator (for AWS Secrets Manager)

```bash
# Install External Secrets Operator
helm repo add external-secrets https://charts.external-secrets.io
helm install external-secrets external-secrets/external-secrets -n external-secrets-system --create-namespace

# Create SecretStore for AWS Secrets Manager
cat <<EOF | kubectl apply -f -
apiVersion: external-secrets.io/v1beta1
kind: SecretStore
metadata:
  name: aws-secretsmanager
  namespace: michelangelo-prod
spec:
  provider:
    aws:
      service: SecretsManager
      region: us-west-2
      auth:
        jwt:
          serviceAccountRef:
            name: michelangelo-worker
EOF
```

## Step 6: Deploy Temporal/Cadence

### Option A: Temporal Cloud (Recommended for Production)

```bash
# Sign up for Temporal Cloud and get connection details
# Update values-prod.yaml with your Temporal Cloud endpoint
```

### Option B: Self-hosted Temporal

```bash
# Install Temporal using Helm
helm repo add temporalio https://go.temporal.io/helm-charts
helm repo update

helm install temporal temporalio/temporal \
  --namespace temporal-system \
  --create-namespace \
  --set server.replicaCount=3 \
  --set cassandra.enabled=false \
  --set mysql.enabled=true \
  --set mysql.persistence.enabled=true \
  --set mysql.persistence.size=100Gi
```

## Step 7: Deploy Michelangelo

### Using Helm (Recommended)

```bash
# Create namespace
kubectl create namespace michelangelo-prod

# Create values file for production
cat > values-aws-prod.yaml <<EOF
global:
  imageRegistry: "ghcr.io"

apiserver:
  replicaCount: 3
  ingress:
    enabled: true
    className: "alb"
    annotations:
      alb.ingress.kubernetes.io/scheme: internet-facing
      alb.ingress.kubernetes.io/target-type: ip
      cert-manager.io/cluster-issuer: letsencrypt-prod
    hosts:
      - host: api.michelangelo.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: michelangelo-api-tls
        hosts:
          - api.michelangelo.yourdomain.com

ui:
  ingress:
    enabled: true
    className: "alb"
    annotations:
      alb.ingress.kubernetes.io/scheme: internet-facing
      alb.ingress.kubernetes.io/target-type: ip
      cert-manager.io/cluster-issuer: letsencrypt-prod
    hosts:
      - host: michelangelo.yourdomain.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: michelangelo-ui-tls
        hosts:
          - michelangelo.yourdomain.com

database:
  external:
    enabled: true
    host: michelangelo-prod-db.region.rds.amazonaws.com
    port: 3306
    database: michelangelo_prod
    username: michelangelo
    existingSecret: michelangelo-database

storage:
  s3:
    enabled: true
    region: us-west-2
    bucket: michelangelo-artifacts-prod-xxxxx
    useIAM: true

workflow:
  external:
    enabled: true
    host: temporal.yourdomain.com
    port: 7233
    domain: michelangelo-prod

monitoring:
  serviceMonitor:
    enabled: true
EOF

# Install Michelangelo
helm install michelangelo-prod ./deploy/production/helm \
  --namespace michelangelo-prod \
  --values values-aws-prod.yaml
```

### Using Kustomize

```bash
# Create production environment configuration
cd deploy/production/environments/prod

# Update kustomization.yaml with AWS-specific settings
# Deploy using kubectl
kubectl apply -k .
```

## Step 8: Verification and Testing

```bash
# Check all pods are running
kubectl get pods -n michelangelo-prod

# Check services
kubectl get services -n michelangelo-prod

# Check ingress
kubectl get ingress -n michelangelo-prod

# Test API health
curl -k https://api.michelangelo.yourdomain.com/health

# Test UI
curl -I https://michelangelo.yourdomain.com
```

## Step 9: Monitoring and Observability

### Install Prometheus and Grafana

```bash
# Add Prometheus community helm repo
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update

# Install Prometheus
helm install prometheus prometheus-community/kube-prometheus-stack \
  --namespace monitoring \
  --create-namespace \
  --set grafana.adminPassword=YourGrafanaPassword \
  --set grafana.ingress.enabled=true \
  --set grafana.ingress.hosts[0]=grafana.yourdomain.com

# Import Michelangelo dashboards
kubectl create configmap michelangelo-dashboards \
  --from-file=deploy/production/monitoring/grafana/dashboards/ \
  -n monitoring
```

## Backup and Disaster Recovery

### Database Backups

```bash
# Enable automated backups in RDS (already configured above)
# Create additional backup strategy
aws events put-rule \
  --name michelangelo-db-backup \
  --schedule-expression "rate(24 hours)"
```

### S3 Cross-Region Replication

```bash
# Set up cross-region replication for S3 bucket
aws s3api put-bucket-replication \
  --bucket michelangelo-artifacts-prod-xxxxx \
  --replication-configuration file://replication-config.json
```

## Security Best Practices

1. **Network Security**
   - Use private subnets for EKS nodes
   - Configure Security Groups restrictively
   - Enable VPC Flow Logs

2. **IAM Security**
   - Use IAM Roles for Service Accounts (IRSA)
   - Follow principle of least privilege
   - Enable CloudTrail logging

3. **Data Encryption**
   - Enable encryption at rest for RDS
   - Use encrypted S3 buckets
   - Enable encryption in transit

4. **Secrets Management**
   - Store secrets in AWS Secrets Manager
   - Use External Secrets Operator
   - Rotate secrets regularly

## Troubleshooting

### Common Issues

1. **EKS Access Denied**
   ```bash
   # Update kubeconfig
   aws eks update-kubeconfig --region us-west-2 --name michelangelo-prod
   ```

2. **RDS Connection Issues**
   ```bash
   # Check security groups
   aws ec2 describe-security-groups --group-ids sg-xxxxxxxxxx
   ```

3. **S3 Permission Issues**
   ```bash
   # Verify IAM policy attachment
   aws iam list-attached-role-policies --role-name eksctl-michelangelo-prod-nodegroup-NodeInstanceRole
   ```

## Scaling

### Horizontal Pod Autoscaler

```bash
# Enable metrics server if not already installed
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

# HPA is configured in Helm values
```

### Cluster Autoscaler

```bash
# Install cluster autoscaler
kubectl apply -f https://raw.githubusercontent.com/kubernetes/autoscaler/master/cluster-autoscaler/cloudprovider/aws/examples/cluster-autoscaler-autodiscover.yaml

# Annotate for your cluster
kubectl -n kube-system annotate deployment.apps/cluster-autoscaler \
  cluster-autoscaler.kubernetes.io/safe-to-evict="false"

kubectl -n kube-system edit deployment.apps/cluster-autoscaler
# Add --node-group-auto-discovery=asg:tag=k8s.io/cluster-autoscaler/enabled,k8s.io/cluster-autoscaler/michelangelo-prod
```

## Cost Optimization

1. **Right-size instances** based on actual usage
2. **Use Spot instances** for worker nodes where appropriate
3. **Enable S3 Intelligent Tiering**
4. **Set up RDS storage autoscaling**
5. **Use Reserved Instances** for predictable workloads

For additional support, see the main [troubleshooting guide](../troubleshooting.md).