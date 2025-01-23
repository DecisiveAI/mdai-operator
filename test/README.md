# Testing

## Fluentd test

```shell
kind create cluster -n mdai-operator-test
```
```shell
kubectl apply -f https://github.com/cert-manager/cert-manager/releases/latest/download/cert-manager.yaml
```
```shell
kubectl apply -f https://github.com/open-telemetry/opentelemetry-operator/releases/latest/download/opentelemetry-operator.yaml
```
```shell
helm install valkey oci://registry-1.docker.io/bitnamicharts/valkey --set auth.password=abc
```
```shell
helm install prometheus prometheus-community/kube-prometheus-stack -f test/test-samples/custom-values.yaml
```
```shell
make install
```
```shell
make deploy
```

* Set replicas operator deployment to 0
* Debug operator with valkey host and password env vars

```shell
kubectl apply -f ./test/test-samples/example_log_generator_xtra_noisy_service.yaml
```
```shell
kubectl apply -f ./test/test-samples/example_log_generator_noisy_service.yaml
```
```shell
kubectl apply -k config/samples/
```
```shell
kubectl apply -k test/test-samples/
```
```shell
helm upgrade --install --repo https://fluent.github.io/helm-charts fluent fluentd -f ./test/test-samples/values_fluentd.yaml
```