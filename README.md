![Coverage](https://img.shields.io/badge/Coverage-1-red)
[![Chores](https://github.com/DecisiveAI/mdai-operator/actions/workflows/chores.yml/badge.svg)](https://github.com/DecisiveAI/mdai-operator/actions/workflows/chores.yml)
# MDAI K8s Operator
manages MDAI Hubs
## Description
MDAI k8s operator: 

- Monitors OTEL collectors with labels matching the hub name.
- Creates alerting rules for the Prometheus operator.
- Reads variables from ValKey.
- Requires environment variables with the ValKey endpoint and password to be provided.
- Supports two types of variables: set and string.
- Converts to uppercase MDAI environment variables when injecting them into the OTEL collector if env variable name is not specified explicitly.
  Injects environment variables into OTEL collectors through a ConfigMap with labels matching the hub name. The OTEL collector must be configured to use the ConfigMap. Operator is not responsible for removing this ConfigMap.
- The ConfigMap name is the MDAI hub name plus `-variables`
```yaml
  envFrom:
    - configMapRef:
      name: mdaihub-sample-variables
```
- For now assuming hub names are unique across all namespaces
- valkey key name has a structure: `variable/some_hub_name/some_variable_name`
- Updates to variables are applied by triggering the collector’s restart
- Supports the built-in ValKey storage type for variables 
- Creates immutable meta variables that have references to other variables

## Getting Started
### Importing opentelemetry-operator module from private repo
1. make sure the following env variable is set
```shell
export GOPRIVATE=github.com/decisiveai/*
```
2. Add the following section to your git client config:
```shell
[url "ssh://git@github.com/"]
	insteadOf = https://github.com/
```

### Prerequisites
- go version v1.22.0+
- docker version 17.03+.
- kubectl version v1.11.3+.
- Access to a Kubernetes v1.11.3+ cluster.
- MDAI OTEL operator CRD installed into cluster.
- Prometheus operator CRD installed into cluster.
- valkey secret created (see below)
- valkey is installed

### To Deploy on the local cluster
**Create cluster & Deploy Cert manager**
```shell
kind create cluster -n mdai-operator-test
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.17.1/cert-manager.yaml
kubectl wait --for=condition=available --timeout=600s deployment --all -n cert-manager
```
**Other prerequisites operator**   
```shell
kubectl apply -f https://github.com/open-telemetry/opentelemetry-operator/releases/latest/download/opentelemetry-operator.yaml
helm install valkey oci://registry-1.docker.io/bitnamicharts/valkey --set auth.password=abc \
  --set image.registry="" \
  --set image.repository=public.ecr.aws/decisiveai/valkey \
  --set image.tag=latest \
  -f test/test-samples/valkey-values.yaml
helm install prometheus prometheus-community/kube-prometheus-stack
helm upgrade prometheus prometheus-community/kube-prometheus-stack -f test/test-samples/prometheus-custom-values.yaml
kubectl create namespace otel
kubectl create namespace mdai
kubectl create configmap mdai-operator-release-info \
  --from-literal=RELEASE_NAME=mdai
  --namespace=mdai
kubectl create secret generic valkey-secret \
  --from-literal=VALKEY_ENDPOINT=valkey-primary.default.svc.cluster.local:6379 \
  --from-literal=VALKEY_PASSWORD=abc \
  --namespace=mdai \
  --dry-run=client -o yaml | kubectl apply -f -
```
**Build and deploy to the local kind cluster:**
Make sure you have the right k8s context selected, run:

```sh
make local-deploy
```

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```
### Testing
Deploy test OTEL collectors:
```sh
kubectl apply -k test/test-samples/
```

>**NOTE**: Ensure that the samples has default values to test it out.

### To Uninstall
**Delete the instances (CRs) from the cluster:**

```sh
kubectl delete -k config/samples/
```
Delete test OTEL collectors:
```sh
kubectl delete -k test/test-samples/
```

**Delete the APIs(CRDs) from the cluster:**

```sh
make uninstall
```

**Undeploy the controller from the cluster:**

```sh
make undeploy
```
## Helm
### Install helm plugin
```shell
make helm-values-schema-json-plugin
```
### Regenerate from the latest manifests:
Update VERSION number in make file and run:
```shell
make helm-update
```
- update chart and app version in `deployment/Chart.yaml`
- update image version in `deployment/values.yaml`
- update `newTag` in `config/manager/kustomization.yaml`
- update `README.md` in `deployment`
- update `values.schema.json` in `deployment`

### Package chart
```shell
make helm-package
```
### Publish chart
```shell
make helm-publish
```

## Project Distribution 

Following the options to release and provide this solution to the users.

### By providing a bundle with all YAML files

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=mdai-operator:v0.0.1
```

**NOTE:** The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without its
dependencies.

2. Using the installer

Users can just run 'kubectl apply -f <URL for YAML BUNDLE>' to install
the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/mdai-operator/<tag or branch>/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

