# What’s a project?

In Michelangelo, a project represents a **business use case** that is continuously measured through a defined set of performance metrics. Examples include **dish recommendations in Uber Eats** or **restaurant ranking on the home feed.** This structure—framing each project as a business use case—has enabled Uber to **drive sustained model evolution within a specific domain over time,** ensuring that improvements are directly tied to measurable business outcomes.

# Create a MA Studio Project from the CLI

To create an MA Studio project using the CLI, follow these steps:

1.  Create a project folder within your repository.
2.  Add the following file to the newly created project folder.

```
<project folder> 
  config/ 
    project.yaml
```

-   Modify the **project.yaml** file with the following example

```apiVersion: michelangelo.api/v2
kind: Project
metadata:
  name: ma-test # k8s naming convention
  namespace: ma-test # k8s naming convention
  annotations:
    michelangelo/worker_queue: [cadence-temporal-worker-queue]
spec:
  description: My Project
  owner:
    owningTeam: "[team-id-used-by-auth]" #LDAP group
    owners:
      - [user-id-used-by-auth]
  tier: 4
  gitRepo: https://[your-github].github.com/[repo-name]
  rootDir: your/ml/root/directory
```
* Run this command to create a new project

```
ma project apply --file="[your-project-folder-root]/project.yaml"
```
