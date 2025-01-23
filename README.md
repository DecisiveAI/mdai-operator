[![E2E Tests](https://github.com/DecisiveAI/mdai-operator/actions/workflows/test-e2e.yml/badge.svg)](https://github.com/DecisiveAI/mdai-operator/actions/workflows/test-e2e.yml)
[![Tests](https://github.com/DecisiveAI/mdai-operator/actions/workflows/test.yml/badge.svg)](https://github.com/DecisiveAI/mdai-operator/actions/workflows/test.yml)
[![Lint](https://github.com/DecisiveAI/mdai-operator/actions/workflows/lint.yml/badge.svg)](https://github.com/DecisiveAI/mdai-operator/actions/workflows/lint.yml)
# mdai-operator
manages MDAI Hub
## Description
Operator 

- Monitors OTEL collectors with labels matching the hub name.
- Creates alerting rules for the Prometheus operator.
- Reads variables from ValKey.
- Requires environment variables with the ValKey endpoint and password to be provided.
- Supports two types of variables: set and scalars.
- Converts to uppercase MDAI environment variables when injecting them into the OTEL collector.
  Injects environment variables into OTEL collectors through a ConfigMap with labels matching the hub name. The OTEL collector must be configured to use the ConfigMap.
- The ConfigMap name is the MDAI hub name plus `-variables`
```yaml
  envFrom:
    - configMapRef:
      name: mdaihub-sample-variabes
```
- Updates to variables are applied by triggering the collector’s restart

- Supports the built-in ValKey storage type for variables 

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

### To run locally
make sure the following env variable is set
```shell
export VALKEY_ENDPOINT=127.0.0.1:6379
export VALKEY_PASSWORD=abc
```

### To Deploy on the cluster
**Generate valkey secret**

```shell
helm install valkey oci://registry-1.docker.io/bitnamicharts/valkey --set auth.password=abc
```

```shell
kubectl create secret generic valkey-secret \
  --from-literal=VALKEY_ENDPOINT=valkey-primary.default.svc.cluster.local:6379 \
  --from-literal=VALKEY_PASSWORD=abc \
  --namespace=mdai-operator-system \
  --dry-run=client -o yaml | kubectl apply -f -
```
**Build and push your image to the location specified by `IMG`:**

```sh
go mod vendor
make docker-build IMG=mdai-operator:v0.0.1
kind load docker-image mdai-operator:v0.0.1 --name mdai-operator-test
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

**Install the CRDs into the cluster:**

```sh
make install
```

**Deploy the Manager to the cluster with the image specified by `IMG`:**

```sh
make deploy IMG=mdai-operator:v0.0.1
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

**Create instances of your solution**
You can apply the samples (examples) from the config/sample:

```sh
kubectl apply -k config/samples/
```
Deploy test otel collectors:

> Requires cert manager and OTEL operator installed
> ```sh
> kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.15.1/cert-manager.yaml
> kubectl apply -f https://github.com/open-telemetry/opentelemetry-operator/releases/latest/download/opentelemetry-operator.yaml
> ```

```sh
kubectl apply -k test/test-samples/
```
### Testing
Add watcher scrape config to Prometheus:

> Requires Prom Kube Stack to be installed
> ```sh
> helm install prometheus prometheus-community/kube-prometheus-stack
> ```

```shell
helm upgrade prometheus prometheus-community/kube-prometheus-stack -f test/test-samples/custom-values.yaml
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

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=mdai-operator:v0.0.1
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the dist directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run kubectl apply -f <URL for YAML BUNDLE> to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/mdai-operator/<tag or branch>/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

