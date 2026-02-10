# Michelangelo UI

Michelangelo's User Interface (UI) provides a standard, code free ML development experience. It guides users through five phases in the ML development lifecycle, from developing your first model to productionization.

## Getting Started

### Deploy to Kubernetes
Production deployment using sandbox manifests as templates for your infrastructure.

- **For: Platform operators**
- **Use case: Full ML platform deployment with UI**

→ **[Deploying Michelangelo UI](./deploying-michelangelo-ui)**

### Integrate with Existing React App
Add Michelangelo components to your existing React application as npm dependencies that connect to your infrastructure.

- **For: Frontend developers, application teams**
- **Use case: Separate frontend/backend infrastructure, or embedding ML capabilities in existing developer tools**

→ **[React Library Integration](./michelangelo-react-library)**

### Local Development
Set up a development environment for contributing to the UI codebase.

- **For: Contributors, UI developers**
- **Use case: UI development and contributions**

→ **[Local Development Setup](./local-development-setup)**

## Architecture

The Michelangelo UI is built with React and communicates with the Michelangelo API server through gRPC-Web. The UI supports two main consumption methods:

- **Containerized deployment**: Complete UI deployed to Kubernetes clusters
- **Component integration**: Individual React components embedded in existing applications

For internal architecture documentation (providers, hooks, types, and component patterns), see the [UI Developer Reference](../../dev/ui/index.md).
