# **Prerequisites** 

## **Required Software** 

This guide assumes you have the following software installed and configured on your system. Please follow the instructions below for each prerequisite.

* [Docker](https://docs.docker.com/get-started/get-docker)  
* [kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)  
* [k3d](https://k3d.io/)

### **Docker** 

Please follow the official Docker installation guide for your operating system: [Official Docker Documentation](https://docs.docker.com/get-started/get-docker)

Alternatively, we can use [Colima](https://github.com/abiosoft/colima) for starting the docker runtime.

Important Configuration: Accessing Your Host from Docker Containers (host.docker.internal)

Docker often requires containers to communicate with services running directly on your host machine (your laptop or development server). To facilitate this, Docker provides a special hostname: host.docker.internal. This name resolves to your host's internal IP address (typically 127.0.0.1).

Verification and Configuration:

1. Open your system's hosts file: Open your terminal and run: sudo nano /etc/hosts (or use your preferred text editor).  
2. Check for the entry: Look for a line similar to:

```
127.0.0.1 host.docker.internal
```

3. Add the entry if missing: If you don't find this line, add it to the end of the file.

Why is this important?

Ensuring this entry exists allows containers managed by Docker (including the Kubernetes nodes created by k3d) to easily connect back to services running on your local development machine using the consistent host.docker.internal address.

### **kubectl** 

kubectl is the command-line tool for interacting with Kubernetes clusters. You will use it to manage and inspect your k3d cluster.

Installation:

Follow the official Kubernetes documentation for installing kubectl: [Install kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl)

```
brew install kubectl
```

### **k3d** 

k3d is a lightweight tool to run local Kubernetes clusters in Docker. It simplifies the process of setting up a Kubernetes environment for development and testing.

Installation:

```
brew install k3d
```

## **GitHub Personal Access Token** 

Michelangelo is not publicly available yet, so we keep Michelangelo's Docker containers in the private GitHub Container Registry, which requires a [GitHub personal access token (classic)](https://github.com/settings/tokens) for authentication.

To enable authentication for the sandbox, please create a GitHub personal access token (classic) with the "read:packages" scope and save it to the CR\_PAT environment variable. For example, you can add the following line to your shell configuration file (such as .bashrc or .zshrc, depending on the shell you use):

```
$ export CR_PAT=your_token_...
$ echo 'export CR_PAT=your_token_...' >> ~/.zshrc
$ source ~/.zshrc

# login before running ma sandbox so that MA docker image can be pulled
$ docker login ghcr.io -u [your github id] -p [CR_PAT] 
```

For a more detailed guide, please refer to [https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry\#authenticating-with-a-personal-access-token-classic](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry#authenticating-with-a-personal-access-token-classic).

Be aware that CR\_PAT environment variable is required while Michelangelo is NOT publicly accessible. Once we become public, the token will no longer be necessary, and this section will be removed.

## **Python Environment** 

This project requires Python version 3.9 or higher to run certain scripts and tools.

Installation:

Please download and install Python 3.9+ from the [official Python downloads page](https://www.python.org/downloads/).

Verification:

Open your terminal or command prompt and verify the installed Python version. The command might vary slightly depending on your system:

python3 \--version  
\# or  
python \--version

The output should display a version number that starts with 3.9 or a higher minor or patch version (e.g., Python 3.9.x, Python 3.10.y).

### **Poetry \- Python Dependency Management** 

Poetry is used to manage the project's Python dependencies, ensuring that you have the correct versions of all necessary libraries for development and running Python-based tools.

Installation:

Follow the official Poetry installation guide for your operating system: [Poetry Installation](https://python-poetry.org/docs/#installing-with-the-official-installer). Michelangelo recommends running the official installer script:

curl \-sSL https://install.python-poetry.org | python3 \-

Make sure to follow the instructions provided during the installation process, which might include adding Poetry's bin directory (e.g., \~/.local/bin on Linux/macOS) to your system's PATH environment variable so you can run the poetry command globally.

Verification:

Open a new terminal or command prompt and check the installed Poetry version:

poetry \--version

This command should output the installed Poetry version number.

Install dependencies

poetry install

This command should install all the dependencies from pyproject.toml.

# **Commands** 

| Action | Description |
| ----- | ----- |
| `create` | Create and start a sandbox cluster |
| `delete` | Delete the sandbox cluster |
| `start` | Start an existing cluster |
| `stop` | Stop a running cluster |
| `demo` | Deploy demo pipelines and resources |

## Create Command Arguments 

### **Command Format** 

```shell
ma sandbox create [OPTIONS]
```

### **Core Options** 

| Argument | Type | Default | Description |
| ----- | ----- | ----- | ----- |
| `--exclude` | List | `[]` | Skip selected services during deployment |
| `--workflow` | Choice | `cadence` | Workflow engine: `cadence` or `temporal` |
| `--create-compute-cluster` | Flag | `False` | Create an additional compute cluster for ray / spark jobs |
| `--compute-cluster-name` | String | `michelangelo-compute-0` | Name of the jobs cluster |
| `--include-experimental` | List | `[]` | Include experimental services |

### **Excludable Services (`--exclude`)** 

* `apiserver`  
* `controllermgr`  
* `ui` (also excludes Envoy)  
* `worker`  
* `ray`  
* `spark`

### **Experimental Services (`--include-experimental`)** 

* `fluent-bit`  
* `mlflow`

## Experimental Features 

### Fluent Bit 

```shell
ma sandbox create --include-experimental fluent-bit
```

* Provides log aggregation and forwarding  
* Installs a DaemonSet \+ ServiceAccount  
* Uses `fluent-bit.yaml` and `fluent-bit-config.yaml`

### MLflow 

```shell
ma sandbox create --include-experimental mlflow
```

* Local experiment tracking server  
* Uses MySQL backend  
* Stores artifacts in MinIO

## Example Configurations

### **Minimal Sandbox** 

```shell
ma sandbox create
```

### **Use Temporal** 

```shell
ma sandbox create --workflow temporal
```

### **No UI** 

```shell
ma sandbox create --exclude ui
```

### **Full Experimental Mode** 

```shell
ma sandbox create \
  --include-experimental fluent-bit mlflow \
  --create-jobs-cluster \
  --workflow temporal
```

### **API-Only Mode** 

```shell
ma sandbox create \
  --exclude ui worker \
  --include-experimental mlflow
```

### **Create a compute Cluster** 

```shell
ma sandbox create \
  --create-compute-cluster \
  --compute-cluster-name my-compute-cluster
```

## Sandbox Management 

### **Delete**

```shell
ma sandbox delete
```

### **Start / Stop** 

```shell
ma sandbox start
ma sandbox stop
```

### **Create Demo Pipelines** 

```shell
ma sandbox demo
```

# **Running Michelangelo Studio UI** 

This section guides you on how to run the Michelangelo Studio user interface locally. Before proceeding, ensure you have completed all the steps in the Prerequisites section.

## **UI Specific Prerequisites** 

Before running the UI, you need to have the following installed:

1. Node.js v22.11.0 (Recommended: Managed with nvm). Follow the instructions in ([Official Node.js Downloads](https://nodejs.org/en/download)). Once installed, use the following commands:

nvm install 22.11.0  
nvm use 22.11.0

Verify with node \--version (should be v22.11.0).

2. Yarn ([Official Yarn Installation](https://yarnpkg.com/getting-started/install))

npm install \--global yarn

Verify with yarn \--version.

### **Sandbox** 

While you can technically run the Michelangelo Studio UI outside Michelangelo's sandbox, your experience will be limited. The UI fetches data from the Michelangelo API, so ensuring the API sandbox environment is running and accessible (as detailed in the "[Running Michelangelo's API sandbox environment](https://www.google.com/search?q=%23running-michelangelos-api-sandbox-environment)" section) is crucial for the UI to function optimally.

Once your API sandbox is set up and running, you can populate it with demo data using the following command:

ma sandbox demo

The previous command will indicate a successful run by outputting the following:

```
Demo CRs created in namespace ma-dev-test.
```

## **Running the Application** 

1. Navigate to the UI directory:  
   cd $REPO\_ROOT/javascript  
2. *(Replace $REPO\_ROOT with the actual path to the root of your clone of michelangelo-ai/michelangelo.)*  
3. Install dependencies: Run the following command to install the necessary JavaScript dependencies for the UI:  
   yarn setup  
4. This command might take a few minutes to complete as it downloads and installs the required packages.  
5. Start the development server: Once the dependencies are installed, start the development server for Michelangelo Studio UI using the following command:  
   yarn dev  
6. You should see output similar to this in your terminal, indicating the server has started:

```
yarn run v1.22.22
$ vite --config app/vite.config.ts
3:02:21 PM [vite] (client) Re-optimizing dependencies because lockfile has changed

  VITE v6.2.5   ready in 187 ms

  ➜  Local:     http://localhost:5173/
  ➜  Network: use --host to expose
  ➜  press h + enter to show help
```

7.   
   Access Michelangelo Studio in your browser:  
   Open your web browser and navigate to http://localhost:5173/. This address is provided in the terminal output after running yarn dev under the "Local" heading, indicating the default address where Michelangelo Studio UI is running.
