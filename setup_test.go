package umbrella

import (
	"strings"
	"testing"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
)

func TestSetup(t *testing.T) {
	c := caddy.NewTestController("dns", "umbrella device_id 0123456789abcdef organization_id 12345678")

	if err := setup(c); err != nil {
		t.Fatal(err)
	}

	plugins := dnsserver.GetConfig(c).Plugin
	if len(plugins) != 1 {
		t.Fatalf("got %d plugins, want 1", len(plugins))
	}

	handler, ok := plugins[0](nil).(*Umbrella)
	if !ok {
		t.Fatalf("got handler type %T, want *Umbrella", plugins[0](nil))
	}
	if handler.Name() != pluginName {
		t.Fatalf("got plugin name %q, want %q", handler.Name(), pluginName)
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  optionTemplate
	}{
		{
			name:  "standard",
			input: "umbrella device_id 0123456789abcdef organization_id 012345678",
			want: newOptionTemplate(12345678, [8]byte{
				0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
			}),
		},
		{
			name:  "reversed properties and zero organization",
			input: "umbrella organization_id 0 device_id FEDCBA9876543210",
			want: newOptionTemplate(0, [8]byte{
				0xfe, 0xdc, 0xba, 0x98, 0x76, 0x54, 0x32, 0x10,
			}),
		},
		{
			name:  "maximum organization",
			input: "umbrella device_id 0000000000000000 organization_id 4294967295",
			want:  newOptionTemplate(4294967295, [8]byte{}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parse(caddy.NewTestController("dns", test.input))
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("got option data %x, want %x", got, test.want)
			}
		})
	}
}

func TestParseRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr string
	}{
		{"missing arguments", "umbrella", "expected device_id and organization_id"},
		{"missing organization", "umbrella device_id 0123456789abcdef", "expected device_id and organization_id"},
		{"missing device", "umbrella organization_id 1", "expected device_id and organization_id"},
		{"extra argument", "umbrella device_id 0123456789abcdef organization_id 1 extra", "expected device_id and organization_id"},
		{"unknown property", "umbrella device 0123456789abcdef organization_id 1", "unknown property"},
		{"duplicate device", "umbrella device_id 0123456789abcdef device_id fedcba9876543210", "device_id may only be specified once"},
		{"duplicate organization", "umbrella organization_id 1 organization_id 2", "organization_id may only be specified once"},
		{"short device", "umbrella device_id 0123456789abcde organization_id 1", "device_id must be 16 hexadecimal characters"},
		{"long device", "umbrella device_id 0123456789abcdef0 organization_id 1", "device_id must be 16 hexadecimal characters"},
		{"invalid device", "umbrella device_id 0123456789abcdeg organization_id 1", "device_id must be 16 hexadecimal characters"},
		{"negative organization", "umbrella device_id 0123456789abcdef organization_id -1", "organization_id must be an unsigned 32-bit decimal integer"},
		{"signed organization", "umbrella device_id 0123456789abcdef organization_id +1", "organization_id must be an unsigned 32-bit decimal integer"},
		{"hexadecimal organization", "umbrella device_id 0123456789abcdef organization_id 0x1", "organization_id must be an unsigned 32-bit decimal integer"},
		{"organization overflow", "umbrella device_id 0123456789abcdef organization_id 4294967296", "organization_id must be an unsigned 32-bit decimal integer"},
		{"block", "umbrella device_id 0123456789abcdef organization_id 1 {\nvalue\n}", "block syntax is not supported"},
		{"repeated directive", "umbrella device_id 0123456789abcdef organization_id 1\numbrella device_id 0123456789abcdef organization_id 1", "can only be used once"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := parse(caddy.NewTestController("dns", test.input))
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("got error %q, want it to contain %q", err, test.wantErr)
			}
		})
	}
}
