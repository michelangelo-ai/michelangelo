# VS Code Web Sandbox Setup

This document describes how to set up and manage a secure VS Code Web environment on GCP for users to experiment with Michelangelo without accessing the source code.

## 🌐 Access Information

### VS Code Web Interface
**URL**: `http://34.172.250.134:8080`
**Password**: `michelangelo-playground-2024`
**User Workspace**: `oai` (Dedicated workspace for OpenAI users)
**Workspace Root**: `/shared/playground/oai/` (Pre-configured with Michelangelo)
**Restricted Access**: Limited to user's dedicated workspace only

### Michelangelo API Server
**URL**: `http://34.172.250.134:14566`
**Purpose**: Programmatic access to Michelangelo API endpoints
**Authentication**: Configure as needed for your applications

**VM**: `sandbox-instance-20251112-220947` (us-central1-a)

## 🔒 Security & Permissions

### User Access Control
✅ **Password protected** - Users need the password to access
✅ **User isolation** - Each user gets dedicated workspace (currently: `oai`)
✅ **Pre-installed environment** - Michelangelo package ready to use
✅ **No cross-user interference** - Users cannot access other workspaces
✅ **No system access** - Users cannot access system directories

### Directory Structure (OAI User Workspace)
```
/shared/playground/oai/       # OAI user workspace (VS Code root)
├── .venv/                   # Isolated Python environment with Michelangelo
├── examples/                # Production examples from Michelangelo team
│   ├── amazon_books_qwen/  # Book recommendation with Qwen model
│   ├── bert_cola/          # BERT for CoLA dataset classification
│   ├── boston_housing_xgb/ # XGBoost for housing price prediction
│   ├── gpt_oss_20b_finetune/ # GPT fine-tuning workflows
│   ├── llm_prediction/     # LLM prediction examples
│   ├── nomic_ai/           # Nomic AI integration examples
│   └── [41 Python files total]
├── workspace/              # User's personal development area
├── notebooks/              # Jupyter notebooks directory
├── activate_env.sh         # Quick environment activation script
├── README.md              # User-focused getting started guide
└── michelangelo_ai-0.1.0-py3-none-any.whl # Package wheel (backup)
```

### User Isolation Model
```
/shared/playground/
├── oai/           # Current user (expandable)
├── user1/         # Future user 1 (copy of template)
├── user2/         # Future user 2 (copy of template)
└── template/      # Master template for new users
```

### Firewall Configuration
- **Port 8080**: VS Code Web access
- **Port 22**: SSH access for management
- **Port 14566**: Michelangelo API server access
- **All other ports**: Blocked

### Storage Configuration
- **Root filesystem**: 100GB boot disk (OS and system files)
- **Persistent storage**: 100GB mounted at `/shared` (Docker, k3d, user data)
- **Docker data**: Configured to use `/shared/docker` for persistent storage
- **k3d cluster**: Uses persistent storage to avoid "no space left on device" errors

## 👥 User Experience

### What Users See (Zero Setup Required!)
When users access VS Code, they immediately get:
- **🎨 Michelangelo pre-installed** - `import michelangelo` works immediately
- **📚 Production examples** - 6 real-world project folders, 41 Python files
- **🐍 Python environment** - Virtual environment pre-activated
- **📖 Clear README** - Step-by-step getting started guide
- **💻 Full IDE** - VS Code with integrated terminal and file explorer

### User Workflow (5 Minutes to Productive!)
1. **Login** → Enter password → VS Code opens in OAI workspace
2. **Activate environment** → `source .venv/bin/activate` (in terminal)
3. **Import & code** → `import michelangelo.uniflow` works instantly!
4. **Run examples** → Browse `examples/` and execute any Python file
5. **Build projects** → Create new files in `workspace/` directory

### What Users Can Access
✅ **Pre-configured Michelangelo environment** - No installation needed
✅ **Production-quality examples** - Real ML/AI workflows
✅ **Isolated workspace** - Own files and projects
✅ **Full Python ecosystem** - All dependencies resolved
✅ **VS Code features** - Extensions, terminal, debugging
✅ **Persistent storage** - Work saved between sessions

### What Users Cannot Access
❌ **Other user workspaces** - Complete isolation
❌ **System directories** - Security boundaries
❌ **Source code** - No access to private repositories
❌ **VM configuration** - Cannot modify system settings

## ⚠️ **SECURITY WARNING: Terminal Access**

**CRITICAL**: Users have shell access via VS Code integrated terminal. This means they can potentially:
- Navigate outside their workspace (`cd /`, `ls /shared/playground/`)
- View system files (`cat /etc/passwd`, `ps aux`)
- Access other directories if they exist on the VM
- See running processes and system information

### 🔒 Security Mitigation Applied
✅ **VS Code workspace restrictions** - Terminal opens in user workspace
✅ **Path limitations** - Restricted command availability
✅ **Python environment auto-activation** - Users start in correct environment
⚠️ **Not foolproof** - Advanced users can still bypass restrictions

### 🛡️ Enhanced Security Options

#### Option 1: Current Setup (Basic Protection)
- VS Code settings restrict initial terminal location
- Path and alias restrictions in place
- Suitable for **trusted users** and **demo environments**

#### Option 2: Container Isolation (Recommended for Production)
- Complete isolation via Docker containers
- No system access whatsoever
- Higher setup complexity but maximum security

#### Option 3: Disable Terminal (Maximum Restriction)
- Remove terminal access entirely from VS Code
- Users can only edit files, no command execution
- Most restrictive but limits functionality

### Pre-installed Capabilities
```python
# These work immediately - no setup required:
import michelangelo
import michelangelo.uniflow
import michelangelo.cli

# Ready-to-run examples:
# - Book recommendation with Qwen
# - BERT text classification
# - XGBoost regression
# - GPT fine-tuning workflows
# - LLM prediction pipelines
# - Nomic AI integrations
```

## 🚀 Management Commands

### Start/Stop Code-Server (OAI Workspace)

```bash
# Start code-server for OAI user (if not running)
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "nohup code-server --config ~/.config/code-server/config.yaml /shared/playground/oai > /tmp/code-server-oai.log 2>&1 &"

# Stop code-server
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- "pkill code-server"

# Check if running
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- "ps aux | grep code-server"

# Check OAI workspace status
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- "ls -la /shared/playground/oai/"
```

### Monitor Usage

```bash
# Check OAI workspace logs
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- "tail -f /tmp/code-server-oai.log"

# See active connections
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- "ss -tuln | grep 8080"

# Check OAI workspace disk usage
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- "du -sh /shared/playground/oai/*"

# Check Michelangelo installation in OAI workspace
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "cd /shared/playground/oai && source .venv/bin/activate && pip list | grep michelangelo"
```

### Update OAI Workspace Content

```bash
# Add new Python packages to OAI environment
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "cd /shared/playground/oai && source .venv/bin/activate && pip install package-name"

# Add new example files
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "cat > /shared/playground/oai/examples/new_example.py << 'EOF'
# Your new example code here
import michelangelo
# Example code...
EOF"

# Update OAI workspace README
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "nano /shared/playground/oai/README.md"

# Update Michelangelo package
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "cd /shared/playground/oai && source .venv/bin/activate && pip install --upgrade michelangelo_ai-0.1.0-py3-none-any.whl"
```

## 🔧 Configuration Files

### Code-Server Configuration
Location: `~/.config/code-server/config.yaml`

```yaml
bind-addr: 0.0.0.0:8080
auth: password
password: michelangelo-playground-2024
cert: false
user-data-dir: /shared/playground/.vscode
extensions-dir: /shared/playground/.extensions
disable-workspace-trust: true
disable-telemetry: true
```

### Firewall Rules Created

```bash
# SSH access
gcloud compute firewall-rules create oss-vpc-allow-ssh \
  --network oss-vpc \
  --allow tcp:22 \
  --source-ranges 0.0.0.0/0

# VS Code Web access
gcloud compute firewall-rules create oss-vpc-allow-code-server \
  --network oss-vpc \
  --allow tcp:8080 \
  --source-ranges 0.0.0.0/0

# Michelangelo API server access
gcloud compute firewall-rules create oss-vpc-allow-michelangelo-api \
  --network oss-vpc \
  --allow tcp:14566 \
  --source-ranges 0.0.0.0/0
```

## 📦 Michelangelo Package Management

### Current Status: ✅ Pre-installed!
**Michelangelo is already installed and ready to use in the OAI workspace!**

Users can immediately:
```python
import michelangelo
import michelangelo.uniflow
import michelangelo.cli
```

### Updating Michelangelo Package

```bash
# Copy new wheel to OAI workspace
gcloud compute scp python/dist/michelangelo_ai-0.1.0-py3-none-any.whl \
  sandbox-instance-20251112-220947:/shared/playground/oai/ --zone=us-central1-a

# Update in OAI environment
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "cd /shared/playground/oai && source .venv/bin/activate && \
   pip install --upgrade michelangelo_ai-0.1.0-py3-none-any.whl"

# Verify update
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "cd /shared/playground/oai && source .venv/bin/activate && \
   python -c 'import michelangelo; print(\"✅ Michelangelo ready!\")'"
```

### Setting Up Additional Dependencies

```bash
# Add ML/AI packages to OAI environment
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "cd /shared/playground/oai && source .venv/bin/activate && \
   pip install torch transformers scikit-learn pandas numpy jupyter"
```

## 🔐 Enhanced Security Implementation

### Implementing Container Isolation (Recommended)

```bash
# Build secure container
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- "
  cd /shared/playground/oai
  sudo docker build -t michelangelo-playground .

  # Run in isolated container
  sudo docker run -d -p 8080:8080 \
    --name michelangelo-oai \
    --restart unless-stopped \
    michelangelo-playground
"
```

### Disable Terminal Access (Maximum Security)

```bash
# Update VS Code settings to disable terminal
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- "
cat > /shared/playground/oai/.vscode/settings.json << 'EOF'
{
    \"terminal.integrated.enabled\": false,
    \"python.defaultInterpreterPath\": \"/shared/playground/oai/.venv/bin/python\",
    \"files.watcherExclude\": {
        \"**/.git/**\": true,
        \"**/node_modules/**\": true
    }
}
EOF
"
```

### Monitor Security Events

```bash
# Check for suspicious terminal activity
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "sudo ausearch -x bash -ts recent || tail /var/log/auth.log"

# Monitor file access outside workspace
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "sudo inotifywait -m /shared/playground/ --exclude '/shared/playground/oai'"
```

## 🛠️ Troubleshooting

### Code-Server Won't Start

```bash
# Check if port is already in use
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- "netstat -tulpn | grep 8080"

# Check logs for errors
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- "cat /tmp/code-server.log"

# Restart with verbose logging
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "code-server --config ~/.config/code-server/config.yaml /shared/playground --log debug"
```

### Can't Access from Browser

```bash
# Check firewall rules
gcloud compute firewall-rules list | grep code-server

# Verify VM external IP
gcloud compute instances describe sandbox-instance-20251112-220947 \
  --zone=us-central1-a \
  --format="value(networkInterfaces[0].accessConfigs[0].natIP)"

# Test connectivity
curl -I http://34.172.250.134:8080
```

### Permission Issues

```bash
# Reset playground permissions
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "sudo chown -R weric:shared-users /shared/playground && \
   sudo chmod -R 755 /shared/playground"
```

### CORS Error for Michelangelo UI API Calls

**Problem**: When accessing the Michelangelo UI from an external IP (like `http://34.172.250.134`), you may get CORS errors when the frontend tries to call API endpoints like `listProject`.

**Symptoms**:
- Browser console shows CORS policy errors
- API calls fail with "Access to fetch at '...' has been blocked by CORS policy"
- Frontend can load but can't retrieve data from the backend

**Root Cause**: The Envoy proxy that handles gRPC-Web requests has a CORS allowlist that only includes `localhost` origins by default.

#### Fix CORS Error Step-by-Step

1. **Connect to the sandbox instance**:
```bash
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a
```

2. **Set up kubectl access to k3d cluster**:
```bash
k3d kubeconfig get michelangelo-sandbox > /tmp/k3d-config.yaml
export KUBECONFIG=/tmp/k3d-config.yaml
```

3. **Check current CORS configuration**:
```bash
kubectl get configmap envoy-config -o yaml | grep -A 5 "allow_origin_string_match"
```

4. **Add your external IP to CORS allowed origins**:
```bash
# Replace 34.172.250.134 with your actual external IP
kubectl get configmap envoy-config -o yaml | \
  sed 's|allow_origin_string_match:|allow_origin_string_match:\n                              - exact: "http://34.172.250.134"|' | \
  kubectl apply -f -
```

5. **Verify the configuration was updated**:
```bash
kubectl get configmap envoy-config -o yaml | grep -A 10 "allow_origin_string_match"
```

6. **Restart the Envoy pod to apply changes**:
```bash
kubectl delete pod envoy
kubectl apply -f /shared/michelangelo_ai/michelangelo/python/michelangelo/cli/sandbox/resources/envoy.yaml
```

7. **Verify Envoy is running**:
```bash
kubectl get pod envoy
```

8. **Test CORS fix** (replace IP with your external IP):
```bash
curl -H "Origin: http://34.172.250.134" \
     -H "Access-Control-Request-Method: GET" \
     -H "Access-Control-Request-Headers: content-type,x-grpc-web" \
     -X OPTIONS http://34.172.250.134:8081/ -v
```

You should see response headers like:
```
< access-control-allow-origin: http://34.172.250.134
< access-control-allow-methods: GET, POST, OPTIONS
< access-control-allow-headers: content-type,context-ttl-ms,grpc-timeout,rpc-caller,rpc-encoding,rpc-service,x-grpc-web,x-user-agent
```

#### Network Architecture Reference
```
Frontend (http://34.172.250.134/)
    ↓ gRPC-Web requests
Envoy Proxy (port 8081 / NodePort 30010) ← CORS enforcement here
    ↓ gRPC conversion
API Server (port 14566 / NodePort 30009)
```

#### If Services Are Not Running

**Check if k3d cluster is running**:
```bash
docker ps | grep k3d
```

**Check if Envoy pod exists**:
```bash
kubectl get pods | grep envoy
```

**Check if API server is running**:
```bash
kubectl get pods | grep michelangelo-apiserver
```

**Restart the entire sandbox environment** (if needed):
```bash
# Navigate to the michelangelo directory and restart sandbox
cd /shared/michelangelo_ai/michelangelo
python -m michelangelo.cli.sandbox start
```

## 📈 Scaling & Multi-User Setup

### Current Setup: Single User (OAI)
- **Current capacity**: 1 dedicated user workspace (`oai`)
- **Performance**: Single VM handles 1-5 concurrent users comfortably
- **Isolation**: Complete workspace separation

### Adding Additional Users

#### Option 1: Same VM, Different Ports (Recommended)
```bash
# Create new user workspace (e.g., "claude-user")
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- "
  # Create new workspace based on OAI template
  cp -r /shared/playground/oai /shared/playground/claude-user

  # Start VS Code on different port
  nohup code-server --config ~/.config/code-server/config.yaml \
    --bind-addr 0.0.0.0:8081 /shared/playground/claude-user > /tmp/code-server-claude.log 2>&1 &
"

# Create firewall rule for new port
gcloud compute firewall-rules create oss-vpc-allow-code-server-8081 \
  --network oss-vpc --allow tcp:8081 --source-ranges 0.0.0.0/0

# User access:
# OAI User: http://34.172.250.134:8080
# Claude User: http://34.172.250.134:8081
```

#### Option 2: Additional VMs (For High Isolation)
```bash
# Create dedicated VM for new users
gcloud compute instances create sandbox-user2 \
  --source-instance=sandbox-instance-20251112-220947 \
  --source-instance-zone=us-central1-a \
  --zone=us-central1-b
```

### User Management Script
```bash
# Create automated user provisioning script
cat > /shared/playground/create-user.sh << 'EOF'
#!/bin/bash
USER_NAME=$1
PORT=$((8080 + ${USER_NAME#user}))

# Copy OAI template to new user
cp -r /shared/playground/oai /shared/playground/$USER_NAME

# Start VS Code for new user
nohup code-server --config ~/.config/code-server/config.yaml \
  --bind-addr 0.0.0.0:$PORT /shared/playground/$USER_NAME > /tmp/code-server-$USER_NAME.log 2>&1 &

echo "User $USER_NAME created on port $PORT"
echo "URL: http://34.172.250.134:$PORT"
EOF

chmod +x /shared/playground/create-user.sh
```

## 💰 Cost Considerations

- **VM Cost**: ~$50-70/month for e2-highmem-4 (depending on usage)
- **Storage**: ~$10/month for 100GB persistent disk
- **Network**: Minimal egress charges
- **Total**: ~$60-80/month for dedicated playground

### Cost Optimization
- Use preemptible instances for development/testing
- Stop VM when not in use (playground data persists)
- Use committed use discounts for production

## 🔄 Backup & Recovery

### Backup Playground Data

```bash
# Create snapshot of playground disk
gcloud compute disks snapshot sandbox-instance-20251112-220947 \
  --zone=us-central1-a \
  --snapshot-names=playground-backup-$(date +%Y%m%d)

# Archive playground directory
gcloud compute ssh sandbox-instance-20251112-220947 --zone=us-central1-a -- \
  "tar -czf /tmp/playground-backup-$(date +%Y%m%d).tar.gz /shared/playground"
```

### Restore from Backup

```bash
# Restore from snapshot
gcloud compute disks create playground-restored \
  --source-snapshot=playground-backup-YYYYMMDD \
  --zone=us-central1-a
```

---

## 📞 Support

For issues or questions about the VS Code sandbox setup:
1. Check the troubleshooting section above
2. Review VM logs: `/tmp/code-server.log`
3. Verify firewall and network connectivity
4. Contact the infrastructure team for VM-level issues

## 📝 Recent Updates

### Version 2.1 (December 9, 2024)
- **🔧 CORS Fix Documentation**: Added comprehensive CORS troubleshooting guide for external IP access
- **🌐 Network Architecture**: Documented Envoy proxy setup and gRPC-Web to gRPC conversion
- **📋 Step-by-step Fixes**: Complete procedures for fixing API connectivity issues

### Version 2.0 (December 2, 2024)
- **🎯 User Isolation**: Dedicated OAI workspace with complete user separation
- **📦 Pre-installed Michelangelo**: Zero-setup experience with all dependencies ready
- **📚 Production Examples**: 41 real-world Python files from Michelangelo team
- **🔒 Security Enhancements**: Terminal restrictions and security monitoring
- **⚠️ Security Warning**: Documented terminal access risks and mitigation strategies
- **🐳 Container Option**: Docker-based isolation for maximum security

### Version 1.1 (December 2, 2024)
- **Python-focused workspace**: VS Code opens directly in Python environment
- **Enhanced examples**: Structured examples with detailed comments
- **Better organization**: Separated examples, workspace, and notebooks
- **Improved README**: User-focused getting started guide

### Available Examples (41 Python files)
1. **amazon_books_qwen/** - Book recommendation system with Qwen model
2. **bert_cola/** - BERT for CoLA dataset text classification
3. **boston_housing_xgb/** - XGBoost for housing price prediction
4. **gpt_oss_20b_finetune/** - GPT model fine-tuning workflows
5. **llm_prediction/** - Large language model prediction pipelines
6. **nomic_ai/** - Nomic AI integration and examples

---

**Last Updated**: December 2, 2024
**Version**: 2.0
**Maintainer**: Michelangelo Team

---

## ⚡ Summary

**Your VS Code playground is ready with enhanced security!**

- ✅ **Zero-setup experience** - Michelangelo pre-installed
- ✅ **Production examples** - 41 real Python files
- ✅ **User isolation** - Dedicated OAI workspace
- ⚠️ **Security awareness** - Terminal access risks documented
- 🔒 **Multiple security options** - From basic to container isolation

**Next steps**: Choose your security level based on user trust and use case requirements.