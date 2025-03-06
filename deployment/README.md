# mdai-operator

![Version: 0.1.7](https://img.shields.io/badge/Version-0.1.7-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.1.7](https://img.shields.io/badge/AppVersion-0.1.7-informational?style=flat-square)

MDAI Operator Helm Chart

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| controllerManager.manager.args[0] | string | `"--metrics-bind-address=:8443"` |  |
| controllerManager.manager.args[1] | string | `"--leader-elect=false"` |  |
| controllerManager.manager.args[2] | string | `"--health-probe-bind-address=:8081"` |  |
| controllerManager.manager.containerSecurityContext.allowPrivilegeEscalation | bool | `false` |  |
| controllerManager.manager.containerSecurityContext.capabilities.drop[0] | string | `"ALL"` |  |
| controllerManager.manager.env.valkeyAuditStreamExpiryMs | string | `"2592000000"` |  |
| controllerManager.manager.image.repository | string | `"public.ecr.aws/p3k6k6h3/mdai-operator"` |  |
| controllerManager.manager.image.tag | string | `"0.1.5"` |  |
| controllerManager.manager.resources.limits.cpu | string | `"500m"` |  |
| controllerManager.manager.resources.limits.memory | string | `"128Mi"` |  |
| controllerManager.manager.resources.requests.cpu | string | `"10m"` |  |
| controllerManager.manager.resources.requests.memory | string | `"64Mi"` |  |
| controllerManager.podSecurityContext.runAsNonRoot | bool | `true` |  |
| controllerManager.replicas | int | `1` |  |
| controllerManager.serviceAccount.annotations | object | `{}` |  |
| kubernetesClusterDomain | string | `"cluster.local"` |  |
| metricsService.ports[0].name | string | `"https"` |  |
| metricsService.ports[0].port | int | `8443` |  |
| metricsService.ports[0].protocol | string | `"TCP"` |  |
| metricsService.ports[0].targetPort | int | `8443` |  |
| metricsService.type | string | `"ClusterIP"` |  |
| webhookService.ports[0].port | int | `443` |  |
| webhookService.ports[0].protocol | string | `"TCP"` |  |
| webhookService.ports[0].targetPort | int | `9443` |  |
| webhookService.type | string | `"ClusterIP"` |  |
| webhooks.autoGenerateCert.certValidDays | int | `3650` |  |
| webhooks.autoGenerateCert.enabled | bool | `true` |  |
| webhooks.certManager.enabled | bool | `false` |  |
