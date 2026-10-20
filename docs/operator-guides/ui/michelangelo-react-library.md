---
last_verified: 2026-10-14
---

# Michelangelo AI React Library

This guide covers integrating Michelangelo AI UI components into existing React applications as npm dependencies.

The Michelangelo AI React Library allows organizations to embed Michelangelo AI Studio capabilities within their existing web applications, maintaining their current deployment infrastructure while adding ML platform functionality.

## Key Concepts

- **Dependency Injection**: Pattern for providing external dependencies (themes, services) to components
- **RPC Client**: TypeScript client that exchanges proto3 JSON with the Michelangelo AI API through Envoy's gRPC-JSON transcoder

## Architecture

### Components
- **@michelangelo-ai/core**: Main UI components and application logic
- **@michelangelo-ai/rpc**: RPC client (`request`), generated proto types, and transcoder error normalization (`normalizeTranscoderError`)

### Technologies
- **Frontend**: React 18, TypeScript
- **Styling**: BaseUI
- **State Management and API integration**: TanStack Query
- **Communication**: proto3 JSON over HTTP `POST /{service}/{method}`, transcoded to gRPC by Envoy

### Design Decisions
- **Dependency injection**: Allows customization of logging, theming, and services

## Setup

### Prerequisites
- React 18 application
- TypeScript (the Michelangelo AI repository itself builds with TypeScript 5.7)
- A Node.js version supported by your toolchain (the Michelangelo AI repository itself builds with Node.js 24.18.0)

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
npm install react@18 react-dom@18

# Required for styling
npm install baseui@18 styletron-engine-atomic styletron-react

# Required for API connection and state management
npm install @tanstack/react-query@5

# Required for routing
npm install react-router-dom@5 react-router-dom-v5-compat@6
```

3. **Basic integration:**
```tsx
// src/App.tsx
import React from 'react';
import { BrowserRouter, Route, Routes } from 'react-router-dom-v5-compat';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { Client as Styletron } from 'styletron-engine-atomic';
import { Provider as StyletronProvider } from 'styletron-react';
import { CoreApp } from '@michelangelo-ai/core';
import { normalizeTranscoderError, request } from '@michelangelo-ai/rpc';

const engine = new Styletron();
const queryClient = new QueryClient();

// CoreApp requires `error`, `service`, and `theme` dependencies. `theme.icons`
// is explained under "Icon Configuration" below.
const dependencies = {
  error: { normalizeError: normalizeTranscoderError },
  service: { request },
  theme: { icons: {} },
};

export function App() {
  return (
    <StyletronProvider value={engine}>
      <QueryClientProvider client={queryClient}>
        <BrowserRouter>
          <Routes>
            <Route path="/michelangelo/*" element={<CoreApp dependencies={dependencies} />} />
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
  "apiBaseUrl": "https://your-envoy-host.example.com"
}
```

`apiBaseUrl` must be the browser-reachable URL of the Envoy proxy (port 8081 by default), not the API server's gRPC port. `@michelangelo-ai/rpc` fetches `/config.json` at runtime and fails if `apiBaseUrl` is missing.

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

## Configuration

### Basic Configuration

**Simple integration:**

`CoreApp` is the only component intended for external consumption. It requires a `dependencies` prop with `error`, `service`, and `theme` entries (optional: `navigationBar`, `user`):

```tsx
import { CoreApp } from '@michelangelo-ai/core';

function App() {
  return <CoreApp dependencies={dependencies} />;
}
```

### API Integration Options

The Michelangelo AI UI can integrate with your API through two approaches:

#### Option 1: Custom Request Handler (Recommended)
Use your existing API infrastructure:
```tsx
const dependencies = {
  error: { normalizeError: yourErrorNormalizer },
  theme: { icons: customIcons },
  service: {
    // requestId is an RPC name such as 'ListProject'; args is the request
    // message; headers are optional per-request headers (e.g. user identity).
    request: async (requestId: string, args: unknown, headers?: Record<string, string>) => {
      // Use your existing HTTP client, auth, etc., and return the response message.
      return yourApiClient.call(requestId, args, headers);
    },
  },
};
```

#### Option 2: Michelangelo AI RPC Client
Use the provided client, which sends proto3 JSON to Envoy (requires the Envoy proxy):
```tsx
import { normalizeTranscoderError, request } from '@michelangelo-ai/rpc';

const dependencies = {
  error: {
    normalizeError: normalizeTranscoderError, // Maps transcoder HTTP/gRPC errors to ApplicationError
  },
  service: {
    request, // POSTs proto3 JSON to Envoy, which transcodes to gRPC
  },
};
```

**Note:** Option 2 requires an Envoy proxy with the `grpc_json_transcoder` filter to translate JSON-over-HTTP requests to gRPC calls to your Michelangelo AI API server. The Michelangelo AI Helm chart deploys this proxy for you (`envoy.enabled: true`).

#### Envoy Proxy Setup (Required for @michelangelo-ai/rpc)

**Port Configuration:**
- Envoy listens on port 8081 by default (`envoy.port` in the Helm chart)
- The client makes requests to Envoy, not directly to the API server
- Envoy forwards to the Michelangelo AI API server's gRPC port (`apiserver.port`, 15566 by default)

**CORS Configuration:**
Envoy must allow your application's origin. With the Helm chart, set `envoy.corsOrigins` to a regular expression (not an exact-match list):
```yaml
envoy:
  corsOrigins: "https://your-app-domain\\.com|http://localhost:[0-9]+"
```

**Backend Routing and Transcoding:** The chart renders the route, the `michelangelo-apiserver` cluster, and the `grpc_json_transcoder` filter (with an allowlist of services generated into `helm/michelangelo/files/transcoder-services.json`). If you run your own Envoy, copy these from `helm/michelangelo/templates/core/envoy-configmap.yaml`; the UI depends on `always_print_primitive_fields: true` and `always_print_enums_as_ints: false` in the transcoder `print_options`.

**Reference Configuration:** See the [Deploying Michelangelo AI UI](./deploying-michelangelo-ui.md) guide.

### Icon Configuration

The Michelangelo AI UI requires icon mapping through dependency injection.

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
  return <CoreApp dependencies={dependencies} />
}
```

## Troubleshooting

**API connectivity issues:**
- Verify CORS configuration on your API server
- Check network requests in browser dev tools
- Ensure API base URL is correct for your environment

### FAQ

**Q: Can I use this with Next.js?**
A: Michelangelo AI UI is intentionally un-opinionated about web frameworks. While untested, there is no reason Michelangelo AI UI would be incompatible with Next.js.

**Q: What React versions are supported?**
A: React 18 is required.

**Q: Can I customize the routing?**
A: Yes, you can mount Michelangelo AI components at any route and integrate with your existing router.

**Q: Can I use only specific components?**
A: No, `CoreApp` is the only component intended for external consumption and customizable through dependency injection and configuration. If you have an unsupported use case, please submit an issue or propose a solution in PR.
