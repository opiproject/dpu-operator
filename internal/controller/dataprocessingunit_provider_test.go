package controller

import (
	"testing"

	configv1 "github.com/openshift/dpu-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestProviderManagesPlugin(t *testing.T) {
	detection := &configv1.VendorProviderDetection{ProductName: "NVIDIA BlueField-3"}
	providers := []configv1.VendorProvider{{
		Spec: configv1.VendorProviderSpec{Vendor: "nvidia", Detection: detection},
		Status: configv1.VendorProviderStatus{Conditions: []metav1.Condition{{
			Type:   configv1.VendorProviderConditionAvailable,
			Status: metav1.ConditionTrue,
		}}},
	}}
	if !ProviderManagesPlugin(providers, "NVIDIA BlueField-3") {
		t.Fatal("available provider with detection should own the plugin")
	}
	if ProviderManagesPlugin(providers, "Marvell DPU") {
		t.Fatal("a different product must keep the in-tree VSP")
	}
	providers[0].Status.Conditions[0].Status = metav1.ConditionFalse
	if ProviderManagesPlugin(providers, "NVIDIA BlueField-3") {
		t.Fatal("a provider that is not Available must not own the plugin")
	}
	providers[0].Status.Conditions[0].Status = metav1.ConditionTrue
	providers[0].Spec.Detection = nil
	if ProviderManagesPlugin(providers, "NVIDIA BlueField-3") {
		t.Fatal("a provider without detection must not own the plugin")
	}
}
