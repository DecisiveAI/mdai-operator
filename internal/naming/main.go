// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package naming is for determining the names for components (containers, services, ...).
// nolint:mnd
package naming

// ConfigMap builds the name for the config map used in the OpenTelemetryCollector containers.
// The configHash should be calculated using manifestutils.GetConfigMapSHA.
func ConfigMap(otelcol, configHash string) string {
	return DNSName(Truncate("%s-collector-%s", 63, otelcol, configHash[:8]))
}

// ConfigMapVolume returns the name to use for the config map's volume in the pod.
func ConfigMapVolume() string {
	return "otc-internal"
}

// ConfigMapExtra returns the prefix to use for the extras mounted configmaps in the pod.
func ConfigMapExtra(extraConfigMapName string) string {
	return DNSName(Truncate("configmap-%s", 63, extraConfigMapName))
}

// OpenTelemetryCollectorName builds the collector (deployment/daemonset) name based on the instance.
func OpenTelemetryCollectorName(otelcolName string) string {
	return DNSName(Truncate("%s", 63, otelcolName))
}

// HeadlessService builds the name for the headless service based on the instance.
func HeadlessService(otelcol string) string {
	return DNSName(Truncate("%s-headless", 63, Service(otelcol)))
}

// MonitoringService builds the name for the monitoring service based on the instance.
func MonitoringService(otelcol string) string {
	return DNSName(Truncate("%s-monitoring", 63, Service(otelcol)))
}

// ExtensionService builds the name for the extension service based on the instance.
func ExtensionService(otelcol string) string {
	return DNSName(Truncate("%s-extension", 63, Service(otelcol)))
}

// Service builds the service name based on the instance.
func Service(otelcol string) string {
	return DNSName(Truncate("%s-collector", 63, otelcol))
}

// GrpcService builds the name for the grpc service based on the instance.
func GrpcService(otelcol string) string {
	return DNSName(Truncate("%s-grpc", 63, Service(otelcol)))
}

// NonGrpcService builds the name for the grpc service based on the instance.
func NonGrpcService(otelcol string) string {
	return DNSName(Truncate("%s-non-grpc", 63, Service(otelcol)))
}

// Ingress builds the ingress name based on the instance.
func Ingress(otelcol string) string {
	return DNSName(Truncate("%s-ingress", 63, otelcol))
}
