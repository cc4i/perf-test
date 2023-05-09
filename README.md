# Performance Testing


## Description

The performance testing process is to evaluate the performance of the system under different workloads and identify potential bottlenecks that can impact its performance. The following are the objectives of the performance testing process:

- Evaluate the system's performance under different workloads
- Identify potential bottlenecks in the system
- Determine the maximum capacity of the system
- Determine the system's response time and throughput
- Evaluate the system's scalability and stability



## Components

- pt-admin: *Management backend for performance testing.*

- pt-operator: *Operator to schedule performance testing in GKE cluster, aggregate metrics and logs into Cloud Operation Suite.*

- ui: *Web UI to manage and visualize performance testing.*

- tf: *Terraform scripts to provision all resources.*



## Prerequisites
```
Terraform >= 1.3.9
kubectl >= 1.25
Go >= 1.19
Docker >= 20.10
Google Cloud SDK >= 423.0.0 with credentials
```


## Setup
```shell
# 0. Set environment variables
# Change to your project id
export PROJECT_ID=$(gcloud config get-value project)
# Change to your region
export REGION=asia-southeast1

# 1. Build pt-admin image and push to GAR
export PT_ADMIN_IMAGE=${REGION}-docker.pkg.dev/${PROJECT_ID}/performance-testing/pt-admin:latest
cd pt-admin
make docker-buildx IMG=${PT_ADMIN_IMAGE}

# 2. Build pt-operator image and push to GAR
export PT_OPERATOR_IMAGE=${REGION}-docker.pkg.dev/${PROJECT_ID}/performance-testing/pt-operator:latest
cd pt-operator
make docker-buildx IMG=${PT_OPERATOR_IMAGE}

# 3. Provision all resources
cd tf
terraform init
# Input your project id, region and pt-admin image
terraform apply
```


## References
- [Taurus Configuration Syntax](https://gettaurus.org/docs/ConfigSyntax/)



