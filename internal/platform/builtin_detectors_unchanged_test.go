package platform

import (
	"reflect"
	"testing"

	configv1 "github.com/openshift/dpu-operator/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Intel, Marvell, and NetSec detection must not change when no VendorProvider
// declares detection, and an NVIDIA provider must not change it either.
func TestBuiltinDetectorsUnchangedByProviders(t *testing.T) {
	// Each built-in product name maps to the platform directory it had before
	// VendorProvider existed.
	want := map[string]string{
		IntelIpuName:               "intel-ipu",
		"Marvell DPU":              "marvell-dpu",
		"Intel Netsec Accelerator": "intel-netsec",
	}

	mgr := NewDpuDetectorManager(&FakePlatform{})
	builtin := detectorNames(mgr.GetDetectors())

	mgr.SetProviderDetectors(ProviderDetectorsFromList(nil))
	if got := detectorNames(mgr.GetDetectors()); !reflect.DeepEqual(got, builtin) {
		t.Fatalf("no providers: detector names = %v, want %v", got, builtin)
	}
	assertVendorDirectories(t, mgr, want)

	detection := blueFieldDetection()
	list := &configv1.VendorProviderList{Items: []configv1.VendorProvider{{
		Spec: configv1.VendorProviderSpec{Vendor: "nvidia", Detection: &detection},
		Status: configv1.VendorProviderStatus{Conditions: []metav1.Condition{{
			Type: configv1.VendorProviderConditionAvailable, Status: metav1.ConditionTrue,
		}}},
	}}}
	mgr.SetProviderDetectors(ProviderDetectorsFromList(list))
	if got := detectorNames(mgr.GetDetectors()); len(got) != len(builtin)+1 {
		t.Fatalf("with NVIDIA provider: detector names = %v, want %d built-in plus one", got, len(builtin))
	}
	assertVendorDirectories(t, mgr, want)
}

func detectorNames(detectors []VendorDetector) []string {
	names := make([]string, 0, len(detectors))
	for _, d := range detectors {
		names = append(names, d.Name())
	}
	return names
}

func assertVendorDirectories(t *testing.T, mgr *DpuDetectorManager, want map[string]string) {
	t.Helper()
	for product, dir := range want {
		got, err := mgr.GetVendorDirectory(product)
		if err != nil {
			t.Fatalf("GetVendorDirectory(%q): %v", product, err)
		}
		if got != dir {
			t.Errorf("GetVendorDirectory(%q) = %q, want %q", product, got, dir)
		}
	}
}
