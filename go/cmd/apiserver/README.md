# Michelangelo AI API Server

Michelangelo AI API Server is the unified gRPC server for all the Michelangelo AI APIs. It provides following functions:
1. Standard CRUD APIs for all the Michelangelo AI API resource types.
2. Additional APIs may be added to support more complex operations.
3. Manage Michelangelo AI API resource schemas.

   When Michelangelo AI API server starts, it syncs the latest API resource schemas to Kubernetes (register / update / delete schemas when needed).
4. Invoke registered API hooks.
