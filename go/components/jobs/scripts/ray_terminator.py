# Prerquisite :
# Run cerberus command to tunnel to michelangelo-apiserver-<env>. Eg : cerberus -r michelangelo-apiserver-dev-5

import subprocess
import json

env = "dev-5"
namespaces = ["ma-integration-test", "ma-ray"]


def terminateJob(terminateSpec):
    terminateCommand = "yab -vv michelangelo-apiserver-" + env + " michelangelo.api.v2beta1.RayJobService/UpdateRayJob --timeout 100s -p localhost:5435 --request=\'" + json.dumps(
        terminateSpec) + "\' &"
    #print(terminateCommand)
    subprocess.getoutput(terminateCommand)


def deleteJob(deleteRequest):
    deleteCommand = "yab -vv michelangelo-apiserver-" + env + " michelangelo.api.v2beta1.RayJobService/DeleteRayJob --timeout 100s -p localhost:5435 --request=\'" + json.dumps(
        deleteRequest) + "\' &"
    print(deleteCommand)
    #subprocess.getoutput(deleteCommand)


def getRayJobNames(namespace):
    output = subprocess.getoutput("mactl rayjob get -n " + namespace + " -e " +
                                  env)
    names = output.splitlines()
    return names


def generateTerminateSpec(job):
    terminateSpec = {
        "ray_job": {
            "type_meta": {
                "kind": "RayJob",
                "apiVersion": "michelangelo.uber.com/v2beta1"
            },
            "metadata": {
                "name": str(job["metadata"]["name"]),
                "namespace": str(job["metadata"]["namespace"]),
                "resourceVersion": str(job["metadata"]["resourceVersion"]),
            },
            "spec": {
                "termination": {
                    "type": 1
                }
            }
        }
    }
    return terminateSpec


def generateDeleteRequest(job):
    deleteRequest = {
        "name": str(job["metadata"]["name"]),
        "namespace": str(job["metadata"]["namespace"])
    }
    return deleteRequest


def shouldTerminateJob(job):
    if "termination" in job["spec"]:
        print("already terminated, skipping job " + job["metadata"]["name"])
        return False
    return True


def terminatorForNamespace(namespace):
    jobNames = getRayJobNames(namespace)
    i = 0
    for name in jobNames:
        getJobJsonCommand = "mactl rayjob get " + name + " -n " + namespace + " -e " + env + " -o json"
        print("Fetching " + str(i) + " job. : Command: " + getJobJsonCommand)
        output = subprocess.getoutput(getJobJsonCommand)
        outputJson = "\n".join(output.split("\n")[1:])
        try:
            job = json.loads(outputJson)[0]
        except Exception as e:
            print("Termination failed for job " + name + " with error " +
                  str(e))
            continue

        if shouldTerminateJob(job):
            terminateSpec = generateTerminateSpec(job)
            terminateJob(terminateSpec)
        i = i + 1


def terminator():
    for namespace in namespaces:
        print("Processing for namespace: " + str(namespace))
        terminatorForNamespace(namespace)


def main():
    if "dev" not in env or "prod" in env:
        print("Non dev environment, aborting.")
        return
    terminator()


if __name__ == "__main__":
    main()
