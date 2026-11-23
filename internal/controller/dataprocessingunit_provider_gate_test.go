package controller

import (
	"context"
	"testing"

	configv1 "github.com/openshift/dpu-operator/api/v1"
	"github.com/openshift/dpu-operator/internal/platform"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// Intel and Marvell products must keep the in-tree VSP when no VendorProvider
// declares detection for them.
var inTreeProducts = []string{platform.IntelIpuName, "Marvell DPU", "Intel Netsec Accelerator"}

func newProviderGateReconciler(t *testing.T, objs ...client.Object) *DataProcessingUnitReconciler {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := configv1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &DataProcessingUnitReconciler{Client: c, Scheme: scheme}
}

func TestPluginProviderManagedInTreeProductsWithoutProviders(t *testing.T) {
	r := newProviderGateReconciler(t)
	for _, product := range inTreeProducts {
		managed, err := r.pluginProviderManaged(context.Background(), product)
		if err != nil {
			t.Fatalf("%s: %v", product, err)
		}
		if managed {
			t.Errorf("%s: a provider claims the plugin, want the in-tree VSP", product)
		}
	}
}

func TestPluginProviderManagedNVIDIAProviderOnlyClaimsNVIDIA(t *testing.T) {
	detection := &configv1.VendorProviderDetection{ProductName: "NVIDIA BlueField-3"}
	provider := &configv1.VendorProvider{
		ObjectMeta: metav1.ObjectMeta{Name: "nvidia"},
		Spec:       configv1.VendorProviderSpec{Vendor: "nvidia", Detection: detection},
		Status: configv1.VendorProviderStatus{Conditions: []metav1.Condition{{
			Type:   configv1.VendorProviderConditionAvailable,
			Status: metav1.ConditionTrue,
		}}},
	}
	r := newProviderGateReconciler(t, provider)

	for _, product := range inTreeProducts {
		managed, err := r.pluginProviderManaged(context.Background(), product)
		if err != nil {
			t.Fatalf("%s: %v", product, err)
		}
		if managed {
			t.Errorf("%s: NVIDIA provider claimed the in-tree product", product)
		}
	}

	managed, err := r.pluginProviderManaged(context.Background(), "NVIDIA BlueField-3")
	if err != nil {
		t.Fatal(err)
	}
	if !managed {
		t.Error("available NVIDIA provider should claim NVIDIA BlueField-3")
	}
}
