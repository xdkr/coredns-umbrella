package umbrella

import (
	"bytes"
	"testing"
)

func TestOption(t *testing.T) {
	template := newOptionTemplate(12345678, [8]byte{
		0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
	})
	option := template.option([4]byte{192, 168, 1, 55})

	want := []byte{
		0x4f, 0x44, 0x4e, 0x53, 0x01, 0x00,
		0x00, 0x08, 0x00, 0xbc, 0x61, 0x4e,
		0x00, 0x10, 0xc0, 0xa8, 0x01, 0x37,
		0x00, 0x40, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef,
	}

	if option.Code != optionCode {
		t.Fatalf("got option code %d, want %d", option.Code, optionCode)
	}
	if !bytes.Equal(option.Data, want) {
		t.Fatalf("got option data %x, want %x", option.Data, want)
	}
}

func TestOptionDoesNotMutateTemplateOrPreviousOption(t *testing.T) {
	template := newOptionTemplate(1, [8]byte{1, 2, 3, 4, 5, 6, 7, 8})
	first := template.option([4]byte{192, 0, 2, 1})
	second := template.option([4]byte{198, 51, 100, 2})

	if got := first.Data[14:18]; !bytes.Equal(got, []byte{192, 0, 2, 1}) {
		t.Fatalf("first option address changed to %v", got)
	}
	if got := second.Data[14:18]; !bytes.Equal(got, []byte{198, 51, 100, 2}) {
		t.Fatalf("got second option address %v", got)
	}
	if got := template[14:18]; !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Fatalf("template address changed to %v", got)
	}
}
