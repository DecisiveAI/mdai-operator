# Managing Ingress for the MDAI Cluster's OTel Collector 

If the Cluster is deployed in public cloud environment, and telemetry sources reside outside of that cluster, 
you might want to expose your OpenTelemetry Collector outside the Kubernetes cluster. MDAI Ingress controller (a part of MDAI operator)
will help you to automate this process.

## How it works

MDAI Ingress controller watches for linked bundles Otelcol-MdaiIngress CRs and dynamically, based on the current OTel collector configuration creates 
Service and Ingress resources, needed for provisioning AWS LoadBalancer resources (AWS Load Balancer Controller is responsible for that).

One Application Load Balancer (ALB) will be created if there is 1 or more gPRC receiver endpoints in the collector config.

One Network Load Balancer (NLB) will be created if there is 1 or more non-gRPC (HTTP, TCP, UDP) receiver endpoints in the collector config.

## Prerequisites

1. Supported Public CLoud: AWS
2. AWS EKS cluster
3. Installed [AWS Load Balancer Controller](https://docs.aws.amazon.com/eks/latest/userguide/aws-load-balancer-controller.html)
4. Access to your internal or public DNS service to create CNAME records for your Load Balancers endpoints
5. SSL certificate(s), issued for your CNAME hosts, and stored in [AWS certificate manager](https://aws.amazon.com/certificate-manager/)

## Configuration

### Disable ingress management in the original OTel Collector Custom Resource.

To achieve that do not add `spec.ingress` filed in the Otelcol CR spec.


###  Configure MDAI Otel Collector Ingress with MdaiIngress Custom Resource.

Create Custom Resource `MdaiIngress` using [this sample](../config/samples/hub_v1_mdaiingress.yaml).
Your CR name and namespace should be equal to Opentelemetrycollector name and namespace respectively:

```yaml
apiVersion: hub.mydecisive.ai/v1
kind: MdaiIngress
metadata:
  name: gateway # <<< 
  namespace: mdai  # <<<
```

```yaml
apiVersion: opentelemetry.io/vbeta1
kind: OpenTelemetryCollector
metadata:
  name: gateway # <<<
  namespace: mdai # <<<
```

`spec.cloudType` sets the public cloud type. Supported values: `aws`
```yaml
spec:
  cloudType: aws
```

`spec.collectorEndpoints` sets a mapping between receivers names and corresponding DNS names. This is for gRPC receivers only
Say, we have the following Otel collector config:
```yaml
    receivers:
      otlp:
        protocols:
          grpc:
      jaeger:
        protocols:
          grpc:
    exporters:
      debug: {}
    processors:
      batch:
    service:
      pipelines:
        logs:
          receivers:
            - otlp
            - jaeger
          processors:
            - batch
          exporters:
            - debug
```
In order to provide routing to these 2 gRPC receivers with just 1 Application Load Balancer, you need to set the following mapping:

```yaml
spec:
  collectorEndpoints:
    otlp: otlp.mdai.io
    jaeger: jaeger.mdai.io
```

