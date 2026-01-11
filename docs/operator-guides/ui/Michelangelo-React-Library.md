# 1. overview / Introduction

### Purpose
This guide covers integrating Michelangelo UI components into existing React applications as npm dependencies.

### Context
The Michelangelo React Library allows organizations to embed Michelangelo Studio capabilities within their existing web applications, maintaining their current deployment infrastructure while adding ML platform functionality.

### Audience
- Frontend developers working on existing React applications
- Platform operators looking to understand UI integration into their platform

## 2. Key Concepts / Terms

- **Dependency Injection**: Pattern for providing external dependencies (themes, services) to components
- **gRPC Client**: Generated TypeScript client for API communication

## 3. Architecture / Design

### Components
- **@michelangelo-ai/core**: Main UI components and application logic
- **@michelangelo-ai/rpc**: gRPC client and API communication utilities

### Technologies
- **Frontend**: React 18, TypeScript
- **Styling**: BaseUI
- **State Management and API integration**: TanStack Query
- **Communication**: gRPC-Web via generated clients

### Design Decisions
- **Dependency injection**: Allows customization of logging, theming, and services

## 4. Installation / Setup

### Prerequisites
- React 18 application
- TypeScript 4.8+ 
- Node.js 18+ for development

### Installation Steps

1. **Install core package:**
```bash
npm install @michelangelo-ai/core
# or
yarn add @michelangelo-ai/core
```

**Optional RPC integration:**
```bash
npm install @michelangelo-ai/rpc  # Only if using Michelangelo's gRPC client
```

2. **Install peer dependencies:**
```bash
# Core React dependencies
npm install react@18 react-dom

# Required for styling
npm install baseui styletron-engine-atomic styletron-react

# Required for API connection and state management
npm install @tanstack/react-query

# Required for routing
npm install react-router-dom react-router-dom-v5-compat
```

3. **Basic integration:**
```tsx
// src/App.tsx
import React from 'react';
import { BrowserRouter, Route, Routes } from 'react-router-dom-v5-compat';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Client as Styletron } from 'styletron-engine-atomic';
import { Provider as StyletronProvider } from 'styletron-react';
import { MichelangeloStudio } from '@michelangelo-ai/core';

const engine = new Styletron();
const queryClient = new QueryClient();

export function App() {
  return (
    <StyletronProvider value={engine}>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <Routes>
            <Route path="/michelangelo/*" element={<MichelangeloStudio />} />
            <Route path="/" element={<YourExistingApp />} />
          </Routes>
        </BrowserRouter>
      </QueryClientProvider>
    </StyletronProvider>
  );
}
```

### Environment Setup (Only if using @michelangelo-ai/rpc)
 
**Create configuration file:**
```json
// public/config.json
{
  "apiBaseUrl": "https://your-api-server.com:8081"
}
```

**Note:** Custom request handlers don't need this configuration file - they handle API endpoints directly in code.

### Verification

Build and run your application using your normal development workflow:

1. **Build your application:**
```bash
npm run build  # or your custom build command
```

2. **Start your development server:**
```bash
npm start  # or yarn dev, yarn start, etc.
```

3. **Access the integrated UI:**
Navigate to your app and then to `/michelangelo` (or whatever route you configured)

## 5. Configuration

### Basic Configuration

**Simple integration:**
```tsx
import { MichelangeloStudio } from '@michelangelo-ai/core';

function App() {
  return (
    <MichelangeloStudio />
  );
}
```

### API Integration Options

The Michelangelo UI can integrate with your API through two approaches:

#### Option 1: Custom Request Handler (Recommended)
Use your existing API infrastructure:
```tsx
const dependencies = {
  service: {
    request: async (url, options) => {
      // Use your existing HTTP client, auth, etc.
      return fetch(url, {
        ...options,
        headers: {
          ...options.headers,
          Authorization: `Bearer ${yourAuthToken}`,
        },
      });
    },
  },
};
```

#### Option 2: Michelangelo gRPC Client
Use the provided gRPC-Web client (requires Envoy proxy):
```tsx
import { normalizeConnectError, request } from '@michelangelo-ai/rpc';

const dependencies = {
  error: {
    normalizeError: normalizeConnectError, // Optional: provides consistent error formatting
  },
  service: {
    request, // Uses gRPC-Web via Envoy proxy
  },
};
```

**Note:** Option 2 requires an Envoy proxy to translate HTTP requests to gRPC calls to your Michelangelo API server.

#### Envoy Proxy Setup (Required for @michelangelo-ai/rpc)

**Port Configuration:**
- Envoy listens on port 8081 for gRPC-Web requests
- UI makes requests to Envoy, not directly to API server
- Envoy forwards to your Michelangelo API server

**CORS Configuration:**
Envoy must allow your application's origin:
```yaml
cors:
  allow_origin_string_match:
    - exact: "http://localhost:3000"        # Your React app's dev server
    - exact: "https://your-app-domain.com"  # Your production domain
```

**Backend Routing:**
```yaml
# In the route configuration:
route:
  cluster: michelangelo-apiserver  # This name must match below

# In the clusters section:
clusters:
  - name: michelangelo-apiserver  # Must match the route cluster name
    endpoints:
      - endpoint:
          address:
            socket_address:
              address: michelangelo-apiserver  # Your API server service name
              port_value: 8081                # Your API server port
```

**Reference Configuration:** See complete Envoy setup in the [Deploying Michelangelo UI](./deploying-michelangelo-ui) guide.

### Icon Configuration

The Michelangelo UI requires icon mapping through dependency injection.

```tsx
import YourLaunchIcon from '@your-icon-library/launch';
import YourCheckIcon from '@your-icon-library/check-circle';

const customIcons = {
  arrowLaunch: YourLaunchIcon,
  circleCheckFilled: YourCheckIcon,
  // Map other required icons
};

const dependencies = {
  theme: {
    icons: customIcons,
  },
};

function App() {
  return <MichelangeloStudio dependencies={dependencies} />
}
```

## 6. Troubleshooting

**API connectivity issues:**
- Verify CORS configuration on your API server
- Check network requests in browser dev tools
- Ensure API base URL is correct for your environment

### FAQ

**Q: Can I use this with Next.js?**
A: Michelangelo UI is intentionally un-opinionated about web frameworks. While untested, there is no reason Michelangelo UI would be incompatible with Next.js.

**Q: What React versions are supported?**
A: React 18 is required.

**Q: Can I customize the routing?**
A: Yes, you can mount Michelangelo components at any route and integrate with your existing router.

**Q: Can I use only specific components?**
A: No, MichelangeloStudio is the only component intended for external consumption and customizable through dependency injection and configuration. If you have an unsupported use case, please submit an issue or propose a solution in PR.
