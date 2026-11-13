/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// VendorProviderAvailableCondition is set on DpuOperatorConfig when
	// spec.vendor requires an out-of-tree VendorProvider.
	VendorProviderAvailableCondition = "VendorProviderAvailable"

	// VendorProviderConditionAvailable is the condition type on VendorProvider.status.
	VendorProviderConditionAvailable = "Available"

	// VendorNvidia is the vendor token that requires an out-of-tree provider.
	VendorNvidia = "nvidia"
)

// VendorProviderSpec defines the desired state of VendorProvider.
type VendorProviderSpec struct {
	// Vendor is the vendor identity this provider serves (e.g. "nvidia").
	// Must match metadata.name for the nvidia convention.
	Vendor string `json:"vendor"`

	// Capabilities lists what this provider implements (e.g. lifecycle, translate).
	// +optional
	Capabilities []string `json:"capabilities,omitempty"`

	// Endpoint is reserved for a future RPC SPI. Empty means the provider
	// watches the same OPI CRs (CR-watch companion).
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
}

// VendorProviderStatus defines the observed state of VendorProvider.
type VendorProviderStatus struct {
	// Conditions include Available when the companion is ready to serve.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=vp
// +kubebuilder:printcolumn:name="Vendor",type="string",JSONPath=".spec.vendor"
// +kubebuilder:printcolumn:name="Available",type="string",JSONPath=".status.conditions[?(@.type=='Available')].status"

// VendorProvider advertises an out-of-tree vendor companion (e.g. NVIDIA DPF adapter).
// Intel and Marvell do not require a VendorProvider; they use the in-tree VSP path.
type VendorProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VendorProviderSpec   `json:"spec,omitempty"`
	Status VendorProviderStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VendorProviderList contains a list of VendorProvider.
type VendorProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VendorProvider `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VendorProvider{}, &VendorProviderList{})
}
