package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type (
	// CloudProvider represents what Could provider is it .
	// +kubebuilder:validation:Enum=aws
	CloudProvider string
)

const (
	// CLoudProviderAws specifies that an aws specific ingress should be created.
	CloudProviderAws CloudProvider = "aws"
)

// IngressService defines Service properties related to Ingress configuration.
type IngressService struct {
	// Type defines the type of the Service.
	// +kubebuilder:validation:Enum=ClusterIP;NodePort;LoadBalancer;ExternalName
	// +optional
	Type corev1.ServiceType `json:"type"`
	// Annotations to add to service.
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// MdaiIngressSpec defines the desired state of MdaiIngress
type MdaiIngressSpec struct {
	// Supported types are: aws
	CloudType CloudProvider `json:"cloudType"`

	// Annotations to add to ingress.
	// e.g. 'cert-manager.io/cluster-issuer: "letsencrypt"'
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// IngressClassName is the name of an IngressClass cluster resource. Ingress
	// controller implementations use this field to know whether they should be
	// serving this Ingress resource.
	// +optional
	IngressClassName *string `json:"ingressClassName,omitempty"`

	// +optional
	GrpcService *IngressService `json:"grpcService,omitempty"`

	// +optional
	NonGrpcService *IngressService `json:"nonGrpcService,omitempty"`

	// CollectorEndpoints should contain dns names for all collectors endpoints (grpc receivers)
	// mapped to receivers names e.g. :
	// otlp/1: otlp1.grpc.some.domain
	// otlp/2: otlp2.grpc.some.domain
	// +optional
	CollectorEndpoints map[string]string `json:"collectorEndpoints,omitempty"`
}

// MdaiIngressStatus defines the observed state of MdaiIngress
type MdaiIngressStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// MdaiIngress is the Schema for the mdaiingresses API
type MdaiIngress struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MdaiIngressSpec   `json:"spec,omitempty"`
	Status MdaiIngressStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MdaiIngressList contains a list of MdaiIngress
type MdaiIngressList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MdaiIngress `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MdaiIngress{}, &MdaiIngressList{})
}
