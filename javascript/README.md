# JavaScript Frontend

This is the frontend application for Michelangelo.

## Setup

1. **Node.js Version**: Ensure you have Node.js version 22.17.0 installed
2. **Initial Setup**: Run `yarn setup` to install dependencies and generate RPC client files
3. **Development**: Run `yarn dev` to start the development server

## Common Issues

### Node.js Version Mismatch
If you encounter an error about incompatible Node.js version, update the `engines.node` field in `package.json` to match your current Node.js version.

### Port Conflicts
The development server uses port 5173 by default. If this port is in use, Vite will automatically try the next available port (e.g., 5174).

## Scripts

- `yarn setup` - Install dependencies and generate RPC clients
- `yarn dev` - Start development server
- `yarn build` - Build for production
- `yarn lint` - Run linting
- `yarn typecheck` - Run TypeScript type checking
- `yarn test` - Run tests