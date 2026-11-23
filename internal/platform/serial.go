package platform

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

const (
	pciSerialFile = "serial"
	pciVPDFile    = "vpd"
	pciConfigFile = "config"

	pcieCapDSN      uint16 = 0x0003
	pcieExtCapStart        = 0x100
)

// readPCISerial reads a board serial for a PCI address. It prefers the sysfs
// serial file, then the VPD SN keyword, then the PCIe Device Serial Number
// capability. The fixed config offset used for Intel devices is not used here,
// because it is not where this capability sits on every card.
func readPCISerial(pciAddress string) (string, error) {
	return readSerial(filepath.Join("/sys/bus/pci/devices", pciAddress))
}

func readSerial(devDir string) (string, error) {
	for _, name := range []string{pciSerialFile, pciVPDFile, pciConfigFile} {
		b, err := os.ReadFile(filepath.Join(devDir, name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("read %s: %w", name, err)
		}
		switch name {
		case pciSerialFile:
			if s := strings.TrimSpace(string(b)); s != "" {
				return s, nil
			}
		case pciVPDFile:
			if sn, ok := vpdSerial(b); ok && sn != "" {
				return sn, nil
			}
		case pciConfigFile:
			if sn, ok := configDSN(b); ok && sn != "" {
				return sn, nil
			}
		}
	}
	return "", fmt.Errorf("no serial in %s", devDir)
}

func vpdSerial(data []byte) (string, bool) {
	i := 0
	for i < len(data) {
		if data[i] == 0x78 { // end tag
			return "", false
		}
		if data[i]&0x80 == 0 {
			i += 1 + int(data[i]&0x07)
			continue
		}
		if i+2 >= len(data) {
			return "", false
		}
		tag := data[i] & 0x7f
		length := int(data[i+1]) | int(data[i+2])<<8
		i += 3
		if length < 0 || i+length > len(data) {
			return "", false
		}
		if tag == 0x10 { // VPD-R
			if sn, ok := vpdKeywords(data[i : i+length]); ok {
				return sn, true
			}
		}
		i += length
	}
	return "", false
}

func vpdKeywords(data []byte) (string, bool) {
	i := 0
	for i+3 <= len(data) {
		key := string(data[i : i+2])
		n := int(data[i+2])
		i += 3
		if i+n > len(data) {
			return "", false
		}
		if key == "SN" {
			return strings.TrimSpace(string(data[i : i+n])), true
		}
		i += n
	}
	return "", false
}

func configDSN(cfg []byte) (string, bool) {
	off := pcieExtCapStart
	for off+4 <= len(cfg) {
		hdr := binary.LittleEndian.Uint32(cfg[off : off+4])
		if hdr == 0 {
			return "", false
		}
		id := uint16(hdr & 0xffff)
		next := int((hdr >> 20) & 0xfff)
		if id == pcieCapDSN && off+12 <= len(cfg) {
			return strings.ToUpper(hex.EncodeToString(cfg[off+4 : off+12])), true
		}
		if next == 0 || next <= off {
			return "", false
		}
		off = next
	}
	return "", false
}

// pciIsVirtualFunction reports whether the PCI device is an SR-IOV virtual
// function. A VF has a physfn link back to its physical function.
func pciIsVirtualFunction(pciAddress string) bool {
	_, err := os.Lstat(filepath.Join("/sys/bus/pci/devices", pciAddress, "physfn"))
	return err == nil
}

// sanitizeSerial makes a board serial safe to embed in a DataProcessingUnit name.
func sanitizeSerial(serial string) string {
	serial = strings.ToLower(strings.TrimSpace(serial))
	var b strings.Builder
	lastDash := false
	for _, r := range serial {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.'
		if !ok {
			if unicode.IsSpace(r) || r == ':' || r == '_' || r == '-' {
				if b.Len() > 0 && !lastDash {
					b.WriteByte('-')
					lastDash = true
				}
			}
			continue
		}
		b.WriteRune(r)
		lastDash = false
	}
	return strings.Trim(b.String(), "-")
}
