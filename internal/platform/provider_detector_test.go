package platform

import (
	"encoding/binary"
	"testing"

	"github.com/jaypipes/ghw"
	"github.com/jaypipes/pcidb"
	configv1 "github.com/openshift/dpu-operator/api/v1"
	"github.com/openshift/dpu-operator/internal/daemon/plugin"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func testPCI(addr, vendor, device string) ghw.PCIDevice {
	return ghw.PCIDevice{
		Address: addr,
		Vendor:  &pcidb.Vendor{ID: vendor},
		Product: &pcidb.Product{ID: device},
	}
}

func blueFieldDetection() configv1.VendorProviderDetection {
	return configv1.VendorProviderDetection{
		PCIVendorID:         "15b3",
		PCIDeviceIDs:        []string{"a2dc", "a2dd"},
		ExcludePCIDeviceIDs: []string{"c2d3"},
		ProductName:         "NVIDIA BlueField-3",
		PlatformName:        "nvidia-bluefield",
	}
}

func TestProviderDetectorTwoPortsOneDPU(t *testing.T) {
	d := newProviderDetector("nvidia", blueFieldDetection())
	serials := map[string]string{"0000:05:00.0": "MT25066004A1", "0000:05:00.1": "MT25066004A1"}
	d.readSerial = func(addr string) (string, error) { return serials[addr], nil }
	d.isVF = func(string) bool { return false }

	var found []plugin.DpuIdentifier
	for _, addr := range []string{"0000:05:00.0", "0000:05:00.1"} {
		pci := testPCI(addr, "15b3", "a2dc")
		ok, err := d.IsDPU(nil, pci, found)
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			continue
		}
		id, err := d.GetDpuIdentifier(nil, &pci)
		if err != nil {
			t.Fatal(err)
		}
		found = append(found, id)
	}
	if len(found) != 1 {
		t.Fatalf("got %d DPUs, want 1: %v", len(found), found)
	}
	if found[0] != "nvidia-bluefield-mt25066004a1" {
		t.Fatalf("identifier %q", found[0])
	}
}

func TestProviderDetectorSkipsExcludedAndVirtualFunctions(t *testing.T) {
	d := newProviderDetector("nvidia", blueFieldDetection())
	d.readSerial = func(string) (string, error) { return "SN1", nil }
	vfs := map[string]bool{"0000:05:00.2": true}
	d.isVF = func(addr string) bool { return vfs[addr] }

	cases := []struct {
		addr, device string
		want         bool
	}{
		{"0000:05:00.0", "c2d3", false},
		{"0000:05:00.2", "a2dc", false},
		{"0000:05:00.3", "a2dd", true},
		{"0000:06:00.0", "1234", false},
		{"0000:07:00.0", "a2dc", false}, // other vendor below
	}
	for _, tc := range cases {
		vendor := "15b3"
		if tc.addr == "0000:07:00.0" {
			vendor = "8086"
		}
		ok, err := d.IsDPU(nil, testPCI(tc.addr, vendor, tc.device), nil)
		if err != nil {
			t.Fatal(err)
		}
		if ok != tc.want {
			t.Errorf("%s %s: got %v, want %v", tc.addr, tc.device, ok, tc.want)
		}
	}
}

func TestProviderDetectorIsHostSideOnly(t *testing.T) {
	d := newProviderDetector("nvidia", blueFieldDetection())
	ok, err := d.IsDpuPlatform(nil)
	if err != nil || ok {
		t.Fatalf("IsDpuPlatform = %v, %v; want false, nil", ok, err)
	}
}

func TestProviderDetectorsFromListRequiresAvailable(t *testing.T) {
	detection := blueFieldDetection()
	list := &configv1.VendorProviderList{Items: []configv1.VendorProvider{
		{
			Spec: configv1.VendorProviderSpec{Vendor: "nvidia", Detection: &detection},
			Status: configv1.VendorProviderStatus{Conditions: []metav1.Condition{{
				Type: configv1.VendorProviderConditionAvailable, Status: metav1.ConditionFalse,
			}}},
		},
	}}
	if got := ProviderDetectorsFromList(list); len(got) != 0 {
		t.Fatalf("not-available provider produced %d detectors", len(got))
	}
	list.Items[0].Status.Conditions[0].Status = metav1.ConditionTrue
	if got := ProviderDetectorsFromList(list); len(got) != 1 {
		t.Fatalf("available provider produced %d detectors", len(got))
	}
	if got := ProviderDetectorsFromList(nil); got != nil {
		t.Fatal("nil list should produce no detectors")
	}
}

func TestSetProviderDetectorsKeepsBuiltin(t *testing.T) {
	mgr := NewDpuDetectorManager(&FakePlatform{})
	before := len(mgr.GetDetectors())
	mgr.SetProviderDetectors([]VendorDetector{newProviderDetector("nvidia", blueFieldDetection())})
	if got := len(mgr.GetDetectors()); got != before+1 {
		t.Fatalf("detectors = %d, want %d", got, before+1)
	}
	mgr.SetProviderDetectors(nil)
	if got := len(mgr.GetDetectors()); got != before {
		t.Fatalf("after clear, detectors = %d, want %d", got, before)
	}
}

func TestVPDAndConfigSerial(t *testing.T) {
	vpd := []byte{
		0x82, 0x00, 0x00, // id string, empty
		0x90, 0x08, 0x00, // VPD-R, 8 bytes
		'S', 'N', 0x05, 'A', 'B', 'C', '1', '2',
		0x78, // end
	}
	sn, ok := vpdSerial(vpd)
	if !ok || sn != "ABC12" {
		t.Fatalf("vpd serial = %q, %v", sn, ok)
	}

	cfg := make([]byte, 0x100+12)
	binary.LittleEndian.PutUint32(cfg[0x100:], uint32(pcieCapDSN))
	copy(cfg[0x104:], []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08})
	got, ok := configDSN(cfg)
	if !ok || got != "0102030405060708" {
		t.Fatalf("config DSN = %q, %v", got, ok)
	}
}

func TestSanitizeSerial(t *testing.T) {
	if got := sanitizeSerial("MT2506:6004 A1"); got != "mt2506-6004-a1" {
		t.Fatalf("got %q", got)
	}
}
