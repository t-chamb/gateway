// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// PeeringInterfaceSpec defines the desired state of PeeringInterface.
type PeeringInterfaceSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of PeeringInterface. Edit peeringinterface_types.go to remove/update
	Foo string `json:"foo,omitempty"`
}

// PeeringInterfaceStatus defines the observed state of PeeringInterface.
type PeeringInterfaceStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// PeeringInterface is the Schema for the peeringinterfaces API.
type PeeringInterface struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PeeringInterfaceSpec   `json:"spec,omitempty"`
	Status PeeringInterfaceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// PeeringInterfaceList contains a list of PeeringInterface.
type PeeringInterfaceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PeeringInterface `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PeeringInterface{}, &PeeringInterfaceList{})
}
