# Michelangelo API Server

Michelangelo API Server is the unified gRPC server for all the Michelangelo APIs. It provides following functions:
1. Standard CRUD APIs for all the Michelangelo API resource types.
2. Additional APIs may be added to support more complex operations.
3. Manage Michelangelo API resource schemas.

   When Michelangelo API server starts, it syncs the latest API resource schemas to Kubernetes (register / update / delete schemas when needed).
4. Invoke registered API hooks.
