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

	// Detection declares the host PCI devices that this provider's product is
	// found on. When set, the daemon creates a host DataProcessingUnit for each
	// matching device, and the provider owns the vendor plugin for that product:
	// the operator does not render a VSP pod for it. Only an Available
	// provider contributes detection.
	// +optional
	Detection *VendorProviderDetection `json:"detection,omitempty"`
}

// VendorProviderDetection describes how a host finds DPUs of one product.
// Detection is host side only, and devices are de-duplicated by board serial,
// so a multi-port card produces one DataProcessingUnit.
type VendorProviderDetection struct {
	// PCIVendorID is the four-hex-digit PCI vendor ID, for example "15b3".
	// +kubebuilder:validation:Pattern=`^[0-9a-fA-F]{4}$`
	PCIVendorID string `json:"pciVendorID"`

	// PCIDeviceIDs are the PCI device IDs that identify a DPU of this product.
	// SR-IOV virtual functions are never counted, whatever their device ID.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Pattern=`^[0-9a-fA-F]{4}$`
	PCIDeviceIDs []string `json:"pciDeviceIDs"`

	// ExcludePCIDeviceIDs are device IDs with the same vendor that are not DPUs,
	// for example an on-card management function.
	// +optional
	// +kubebuilder:validation:items:Pattern=`^[0-9a-fA-F]{4}$`
	ExcludePCIDeviceIDs []string `json:"excludePCIDeviceIDs,omitempty"`

	// ProductName is the DataProcessingUnit spec.dpuProductName for these devices.
	// +kubebuilder:validation:MinLength=1
	ProductName string `json:"productName"`

	// PlatformName prefixes the DataProcessingUnit name, which is
	// <platformName>-<lowercase serial>-host. It must be a lowercase DNS label.
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	// +kubebuilder:validation:MaxLength=40
	PlatformName string `json:"platformName"`
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

// Available reports whether the companion has set the Available condition to True.
func (vp *VendorProvider) Available() bool {
	if vp == nil {
		return false
	}
	for _, c := range vp.Status.Conditions {
		if c.Type == VendorProviderConditionAvailable && c.Status == metav1.ConditionTrue {
			return true
		}
	}
	return false
}
