## 1. Overview / Introduction

### Purpose
This guide covers setting up a local development environment for contributing to the Michelangelo UI codebase. The local development setup provides hot-reload capabilities, debugging tools, and integration with the Michelangelo sandbox environment for UI development and testing.

### Audience
- Contributors to the Michelangelo UI

## 2. Key Concepts / Terms

**Hot Reload**: Automatic browser refresh when code changes are detected
**Vite**: Modern build tool providing fast development server and optimized builds
**Sandbox Environment**: Local Kubernetes cluster with all Michelangelo components
**gRPC-Web**: Protocol enabling browser-based gRPC communication
**Protobuf Generation**: Creating TypeScript clients from .proto files
**Yarn Workspaces**: Monorepo management tool for handling multiple packages

## 3. Architecture / Design

### Components
- **Vite Development Server**: Fast development server with hot reload
- **Sandbox Integration**: Local Kubernetes cluster with API services
- **gRPC Client Generation**: Automated TypeScript client creation
- **Yarn Workspace**: Monorepo structure for packages

### Technologies
- **Build Tool**: Vite 6.2+
- **Package Manager**: Yarn (required for workspaces)
- **Development**: Node.js 22.11.0
- **Protobuf**: buf CLI for code generation
- **Container**: Docker for sandbox environment

## 4. Installation / Setup

### Prerequisites

**Node.js 22.11.0:**
```bash
# Using nvm (recommended)
nvm install 22.11.0
nvm use 22.11.0

# Verify installation
node --version  # Should output v22.11.0
```

**Yarn:**
```bash
npm install --global yarn
yarn --version  # Verify installation
```

**For full-stack development (optional):** following [Michelangelo Sandbox Getting Started](../../setup-guide/Sandbox-Setup).

**Note:** The sandbox is optional for UI development. You can develop UI components and pages without a running API server by mocking responses or working with static data.

### Installation Steps

1. **Clone repository:**
```bash
git clone https://github.com/michelangelo-ai/michelangelo.git
cd michelangelo
```

2. **Install dependencies:**
```bash
cd javascript
yarn setup
```

3. [Optional] **Start sandbox environment:** 
```bash
# In separate terminal
ma sandbox create
ma sandbox demo  # Optional: add demo data
```

4. **Start development server:**
```bash
yarn dev
```

5. **Access development UI:**
Open http://localhost:5173

### Verification

1. **Development server starts:**
```
VITE v6.2.5   ready in 187 ms

➜  Local:     http://localhost:5173/
➜  Network: use --host to expose
```

2. [Optional] **API connectivity:**
- Open http://localhost:5173
- Check browser dev tools → Network tab
- Verify API calls to localhost:8081 succeed


### FAQ

**Q: Can I use npm instead of yarn?**
A: No, the project requires Yarn workspaces for monorepo management.

**Q: Why does the first build take so long?**
A: Initial dependency installation and gRPC client generation. Subsequent builds are much faster.

**Q: Can I develop without the sandbox?**
A: Yes, you can work with mock data or specifically on data-independent functionality.

## References & Further Reading

- [Vite Documentation](https://vitejs.dev/)
- [Yarn Workspaces Guide](https://classic.yarnpkg.com/en/docs/workspaces/)
- [Node Version Manager (nvm)](https://github.com/nvm-sh/nvm)
- [Michelangelo Sandbox Documentation](../../setup-guide/Sandbox-Setup)