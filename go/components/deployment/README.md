# Local Development (Mac)
1. Go to controller directory
   ```
    cd src/code.uber.internal/uberai/michelangelo/controllermgr
   ```
2. Ensure that Docker Desktop is running, then setup dependencies. The important components that are started here are the tunnels and minikube
   ```
    ./hack/setup-all.sh 
   ```
3. Make sure that the CRDs are registered
   ```
    make manifests
    make install
   ```
4. Start deployment controller
   ```
    CONTROLLERS="inferenceServer,deployment" ./hack/start-controllermgr.sh 
   ```

## Common commands
1. Create/Update deployment
    ```
    kubectl apply -f ./manifest/samples/v2beta1_deployment.yaml
    ```
2. Get all deployments
    ```
    kubectl describe ModelDeployment
    ```
3. Delete deployment
    ```
    kubectl delete -f ./manifest/samples/v2beta1_deployment.yaml
    ```
