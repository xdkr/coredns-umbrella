package umbrella

import (
	"bytes"
	"net/netip"
	"testing"
)

func TestOption(t *testing.T) {
	template := newOptionTemplate(12345678, [8]byte{
		0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
	})
	tests := []struct {
		name    string
		address string
		want    []byte
	}{
		{
			name:    "ipv4",
			address: "192.168.1.55",
			want: []byte{
				0x4f, 0x44, 0x4e, 0x53, 0x01, 0x00,
				0x00, 0x08, 0x00, 0xbc, 0x61, 0x4e,
				0x00, 0x10, 0xc0, 0xa8, 0x01, 0x37,
				0x00, 0x40, 0x01, 0x23, 0x45, 0x67,
				0x89, 0xab, 0xcd, 0xef,
			},
		},
		{
			name:    "ipv6",
			address: "fe80::0202:b3ff:fe1e:8329",
			want: []byte{
				0x4f, 0x44, 0x4e, 0x53, 0x01, 0x00,
				0x00, 0x08, 0x00, 0xbc, 0x61, 0x4e,
				0x00, 0x20, 0xfe, 0x80, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x02, 0x02,
				0xb3, 0xff, 0xfe, 0x1e, 0x83, 0x29,
				0x00, 0x40, 0x01, 0x23, 0x45, 0x67,
				0x89, 0xab, 0xcd, 0xef,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			option := template.option(netip.MustParseAddr(test.address))
			if option.Code != optionCode {
				t.Fatalf("got option code %d, want %d", option.Code, optionCode)
			}
			if !bytes.Equal(option.Data, test.want) {
				t.Fatalf("got option data %x, want %x", option.Data, test.want)
			}
		})
	}
}

func TestOptionDoesNotMutateTemplateOrPreviousOption(t *testing.T) {
	template := newOptionTemplate(1, [8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	original := template
	first := template.option(netip.MustParseAddr("192.0.2.1"))
	second := template.option(netip.MustParseAddr("2001:db8::2"))
	third := template.option(netip.MustParseAddr("198.51.100.3"))

	if got := first.Data[14:18]; !bytes.Equal(got, []byte{192, 0, 2, 1}) {
		t.Fatalf("first option address changed to %v", got)
	}
	if got := second.Data[14:30]; !bytes.Equal(got, netip.MustParseAddr("2001:db8::2").AsSlice()) {
		t.Fatalf("got second option address %v", got)
	}
	if got := third.Data[14:18]; !bytes.Equal(got, []byte{198, 51, 100, 3}) {
		t.Fatalf("got third option address %v", got)
	}
	if template != original {
		t.Fatal("mutated option template")
	}
}
