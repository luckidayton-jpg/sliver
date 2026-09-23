package evilginx

import (
	"debug/pe"
	"encoding/binary"
	"testing"
)

// minimalPE builds a tiny in-memory PE image with the given machine type
// and DLL flag — just enough for debug/pe to parse the COFF header.
func minimalPE(machine uint16, dll bool) []byte {
	buf := make([]byte, 0x200)
	buf[0], buf[1] = 'M', 'Z'
	binary.LittleEndian.PutUint32(buf[0x3c:], 0x80) // e_lfanew
	copy(buf[0x80:], []byte{'P', 'E', 0, 0})
	binary.LittleEndian.PutUint16(buf[0x84:], machine)
	var chars uint16 = 0x010F // executable, 32-bit-ish
	if dll {
		chars |= pe.IMAGE_FILE_DLL
	}
	binary.LittleEndian.PutUint16(buf[0x96:], chars)  // COFF Characteristics
	binary.LittleEndian.PutUint16(buf[0x98:], 0x10b)  // Optional header magic (PE32)
	return buf
}

func TestValidateSpoofDonor(t *testing.T) {
	exe := minimalPE(pe.IMAGE_FILE_MACHINE_AMD64, false)
	dll := minimalPE(pe.IMAGE_FILE_MACHINE_AMD64, true)
	i386 := minimalPE(pe.IMAGE_FILE_MACHINE_I386, false)

	base := ImplantParams{GOOS: "windows", GOARCH: "amd64", Format: "executable"}
	if err := validateSpoofDonor(base, exe); err != nil {
		t.Fatalf("valid exe donor rejected: %v", err)
	}

	shared := ImplantParams{GOOS: "windows", GOARCH: "amd64", Format: "shared"}
	if err := validateSpoofDonor(shared, dll); err != nil {
		t.Fatalf("valid dll donor rejected: %v", err)
	}
	thirdParty := ImplantParams{GOOS: "windows", GOARCH: "amd64", Format: "third-party"}
	if err := validateSpoofDonor(thirdParty, dll); err != nil {
		t.Fatalf("valid third-party dll donor rejected: %v", err)
	}

	cases := []struct {
		name   string
		params ImplantParams
		donor  []byte
	}{
		{"dll for exe", base, dll},
		{"exe for shared", shared, exe},
		{"wrong arch", base, i386},
		{"not a pe", base, []byte("hello world, not an executable ............ padding to 64 bytes................")},
		{"empty donor", base, nil},
		{"non-windows target", ImplantParams{GOOS: "linux", GOARCH: "amd64", Format: "executable"}, exe},
		{"shellcode target", ImplantParams{GOOS: "windows", GOARCH: "amd64", Format: "shellcode"}, exe},
		{"bad arch", ImplantParams{GOOS: "windows", GOARCH: "mips", Format: "executable"}, exe},
	}
	for _, tc := range cases {
		if err := validateSpoofDonor(tc.params, tc.donor); err == nil {
			t.Errorf("%s: expected rejection, got accept", tc.name)
		}
	}
}
