package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// MdaiReplaySpec defines the desired state of MdaiReplay.
type MdaiReplaySpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of MdaiReplay. Edit mdaireplay_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// MdaiReplayStatus defines the observed state of MdaiReplay.
type MdaiReplayStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// MdaiReplay is the Schema for the mdaireplays API.
type MdaiReplay struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MdaiReplaySpec   `json:"spec,omitempty"`
	Status MdaiReplayStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// MdaiReplayList contains a list of MdaiReplay.
type MdaiReplayList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MdaiReplay `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MdaiReplay{}, &MdaiReplayList{})
}
