package platform

import (
	"fmt"
	"strings"

	"github.com/jaypipes/ghw"
	configv1 "github.com/openshift/dpu-operator/api/v1"
	"github.com/openshift/dpu-operator/internal/daemon/plugin"
	"github.com/openshift/dpu-operator/internal/images"
	"github.com/openshift/dpu-operator/internal/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// providerDetector finds host DPUs described by a VendorProvider. It never
// reports a DPU-side platform: the provider owns that side and deploys the plugin.
type providerDetector struct {
	vendor     string
	detection  configv1.VendorProviderDetection
	readSerial func(pciAddress string) (string, error)
	isVF       func(pciAddress string) bool
}

func newProviderDetector(vendor string, detection configv1.VendorProviderDetection) *providerDetector {
	return &providerDetector{
		vendor:     vendor,
		detection:  detection,
		readSerial: readPCISerial,
		isVF:       pciIsVirtualFunction,
	}
}

// ProviderDetectorsFromList returns one detector per Available provider that
// declared detection. Providers that are missing, not Available, or have no
// detection are ignored, which leaves Intel and Marvell on the built-in path.
func ProviderDetectorsFromList(list *configv1.VendorProviderList) []VendorDetector {
	if list == nil {
		return nil
	}
	var out []VendorDetector
	for i := range list.Items {
		vp := &list.Items[i]
		if vp.Spec.Detection == nil || !vp.Available() {
			continue
		}
		out = append(out, newProviderDetector(vp.Spec.Vendor, *vp.Spec.Detection))
	}
	return out
}

func (d *providerDetector) Name() string { return d.detection.ProductName }

func (d *providerDetector) GetVendorName() string { return d.vendor }

func (d *providerDetector) DpuPlatformName() string { return d.detection.PlatformName }

func (d *providerDetector) IsDpuPlatform(Platform) (bool, error) { return false, nil }

func (d *providerDetector) DpuPlatformIdentifier(Platform) (plugin.DpuIdentifier, error) {
	return plugin.DpuIdentifier(d.detection.PlatformName), nil
}

func (d *providerDetector) VspPlugin(dpuMode bool, imageManager images.ImageManager, c client.Client, pm utils.PathManager, dpuIdentifier plugin.DpuIdentifier) (*plugin.GrpcPlugin, error) {
	return plugin.NewGrpcPlugin(dpuMode, dpuIdentifier, c, plugin.WithPathManager(pm))
}

func (d *providerDetector) IsDPU(_ Platform, pci ghw.PCIDevice, dpuDevices []plugin.DpuIdentifier) (bool, error) {
	if pci.Vendor == nil || pci.Product == nil {
		return false, nil
	}
	if !strings.EqualFold(pci.Vendor.ID, d.detection.PCIVendorID) {
		return false, nil
	}
	if pciIDIn(pci.Product.ID, d.detection.ExcludePCIDeviceIDs) {
		return false, nil
	}
	if !pciIDIn(pci.Product.ID, d.detection.PCIDeviceIDs) {
		return false, nil
	}
	if d.isVF != nil && d.isVF(pci.Address) {
		return false, nil
	}
	id, err := d.GetDpuIdentifier(nil, &pci)
	if err != nil {
		// Skip a device whose serial cannot be read. Failing the whole scan
		// would drop every other DPU on the node.
		return false, nil
	}
	for _, existing := range dpuDevices {
		if existing == id {
			return false, nil
		}
	}
	return true, nil
}

func (d *providerDetector) GetDpuIdentifier(_ Platform, pci *ghw.PCIDevice) (plugin.DpuIdentifier, error) {
	if pci == nil {
		return "", fmt.Errorf("nil PCI device")
	}
	serial, err := d.readSerial(pci.Address)
	if err != nil {
		return "", err
	}
	serial = sanitizeSerial(serial)
	if serial == "" {
		return "", fmt.Errorf("empty serial for %s", pci.Address)
	}
	return plugin.DpuIdentifier(d.detection.PlatformName + "-" + serial), nil
}

func pciIDIn(id string, ids []string) bool {
	for _, candidate := range ids {
		if strings.EqualFold(candidate, id) {
			return true
		}
	}
	return false
}
