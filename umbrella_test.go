package umbrella

import (
	"bytes"
	"context"
	"errors"
	"net"
	"reflect"
	"testing"

	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	plugintest "github.com/coredns/coredns/plugin/test"
	"github.com/miekg/dns"
)

func TestServeDNSInjectsOption(t *testing.T) {
	query := newQuery()
	original := packed(t, query)
	wantErr := errors.New("next")
	var forwarded *dns.Msg

	u := &Umbrella{
		Next: plugin.HandlerFunc(func(_ context.Context, _ dns.ResponseWriter, request *dns.Msg) (int, error) {
			forwarded = request
			return dns.RcodeNameError, wantErr
		}),
		option: newOptionTemplate(12345678, [8]byte{
			0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
		}),
	}

	rcode, err := u.ServeDNS(context.Background(), &plugintest.ResponseWriter{RemoteIP: "192.168.1.55"}, query)
	if rcode != dns.RcodeNameError {
		t.Fatalf("got rcode %d, want %d", rcode, dns.RcodeNameError)
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}
	if forwarded == query {
		t.Fatal("forwarded the original request")
	}
	if !bytes.Equal(packed(t, query), original) {
		t.Fatal("mutated the original request")
	}

	opt := forwarded.IsEdns0()
	if opt == nil {
		t.Fatal("forwarded request has no OPT record")
	}
	if opt.UDPSize() != dns.MinMsgSize {
		t.Fatalf("got UDP size %d, want %d", opt.UDPSize(), dns.MinMsgSize)
	}
	if opt.Do() {
		t.Fatal("set the DO bit")
	}
	if len(opt.Option) != 1 {
		t.Fatalf("got %d options, want 1", len(opt.Option))
	}

	want := []byte{
		0x4f, 0x44, 0x4e, 0x53, 0x01, 0x00,
		0x00, 0x08, 0x00, 0xbc, 0x61, 0x4e,
		0x00, 0x10, 0xc0, 0xa8, 0x01, 0x37,
		0x00, 0x40, 0x01, 0x23, 0x45, 0x67,
		0x89, 0xab, 0xcd, 0xef,
	}
	local, ok := opt.Option[0].(*dns.EDNS0_LOCAL)
	if !ok {
		t.Fatalf("got option type %T, want *dns.EDNS0_LOCAL", opt.Option[0])
	}
	if local.Code != optionCode {
		t.Fatalf("got option code %d, want %d", local.Code, optionCode)
	}
	if !bytes.Equal(local.Data, want) {
		t.Fatalf("got option data %x, want %x", local.Data, want)
	}
}

func TestServeDNSPreservesEDNSAndReplacesUmbrellaOptions(t *testing.T) {
	query := newQuery()
	query.SetEdns0(1232, true)
	query.IsEdns0().Option = []dns.EDNS0{
		&dns.EDNS0_LOCAL{Code: 65001, Data: []byte{1, 2, 3}},
		&dns.EDNS0_SUBNET{
			Code:          dns.EDNS0SUBNET,
			Family:        1,
			SourceNetmask: 24,
			Address:       net.IPv4(198, 51, 100, 0),
		},
		&dns.EDNS0_LOCAL{Code: optionCode, Data: []byte{4}},
		&dns.EDNS0_LOCAL{Code: optionCode, Data: []byte{5}},
	}
	original := packed(t, query)
	var forwarded *dns.Msg

	u := &Umbrella{
		Next: plugin.HandlerFunc(func(_ context.Context, _ dns.ResponseWriter, request *dns.Msg) (int, error) {
			forwarded = request
			return dns.RcodeSuccess, nil
		}),
		option: newOptionTemplate(1, [8]byte{1, 2, 3, 4, 5, 6, 7, 8}),
	}

	if _, err := u.ServeDNS(context.Background(), &plugintest.ResponseWriter{RemoteIP: "203.0.113.9"}, query); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(packed(t, query), original) {
		t.Fatal("mutated the original request")
	}

	opt := forwarded.IsEdns0()
	if opt == nil {
		t.Fatal("forwarded request has no OPT record")
	}
	if opt.UDPSize() != 1232 {
		t.Fatalf("got UDP size %d, want 1232", opt.UDPSize())
	}
	if !opt.Do() {
		t.Fatal("cleared the DO bit")
	}
	if got, want := optionCodes(opt), []uint16{65001, dns.EDNS0SUBNET, optionCode}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got option codes %v, want %v", got, want)
	}
	if local := opt.Option[0].(*dns.EDNS0_LOCAL); !bytes.Equal(local.Data, []byte{1, 2, 3}) {
		t.Fatalf("changed unrelated option data to %x", local.Data)
	}
	subnet, ok := opt.Option[1].(*dns.EDNS0_SUBNET)
	if !ok || subnet.Family != 1 || subnet.SourceNetmask != 24 || subnet.SourceScope != 0 || !subnet.Address.Equal(net.IPv4(198, 51, 100, 0)) {
		t.Fatalf("changed ECS option to %#v", opt.Option[1])
	}
	if got := opt.Option[2].(*dns.EDNS0_LOCAL).Data[14:18]; !bytes.Equal(got, []byte{203, 0, 113, 9}) {
		t.Fatalf("got client address %v", got)
	}
}

func TestServeDNSInjectsIPv6Option(t *testing.T) {
	tests := []struct {
		name string
		tcp  bool
	}{
		{"udp", false},
		{"tcp", true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := newQuery()
			var forwarded *dns.Msg
			u := &Umbrella{
				Next: plugin.HandlerFunc(func(_ context.Context, _ dns.ResponseWriter, request *dns.Msg) (int, error) {
					forwarded = request
					return dns.RcodeSuccess, nil
				}),
				option: newOptionTemplate(12345678, [8]byte{
					0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
				}),
			}
			writer := &plugintest.ResponseWriter{
				TCP:      test.tcp,
				RemoteIP: "fe80::0202:b3ff:fe1e:8329",
				Zone:     "eth0",
			}

			if _, err := u.ServeDNS(context.Background(), writer, query); err != nil {
				t.Fatal(err)
			}
			if forwarded == query {
				t.Fatal("forwarded the original request")
			}
			if query.IsEdns0() != nil {
				t.Fatal("mutated the original request")
			}

			opt := forwarded.IsEdns0()
			if opt == nil || len(opt.Option) != 1 {
				t.Fatalf("got forwarded OPT record %v", opt)
			}
			local, ok := opt.Option[0].(*dns.EDNS0_LOCAL)
			if !ok {
				t.Fatalf("got option type %T, want *dns.EDNS0_LOCAL", opt.Option[0])
			}
			want := []byte{
				0x4f, 0x44, 0x4e, 0x53, 0x01, 0x00,
				0x00, 0x08, 0x00, 0xbc, 0x61, 0x4e,
				0x00, 0x20, 0xfe, 0x80, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x02, 0x02,
				0xb3, 0xff, 0xfe, 0x1e, 0x83, 0x29,
				0x00, 0x40, 0x01, 0x23, 0x45, 0x67,
				0x89, 0xab, 0xcd, 0xef,
			}
			if local.Code != optionCode || !bytes.Equal(local.Data, want) {
				t.Fatalf("got forwarded option %d:%x, want %d:%x", local.Code, local.Data, optionCode, want)
			}
		})
	}
}

func TestServeDNSIgnoresInvalidClientAddress(t *testing.T) {
	query := newQuery()
	var forwarded *dns.Msg
	u := &Umbrella{
		Next: plugin.HandlerFunc(func(_ context.Context, _ dns.ResponseWriter, request *dns.Msg) (int, error) {
			forwarded = request
			return dns.RcodeSuccess, nil
		}),
		option: newOptionTemplate(1, [8]byte{}),
	}

	if _, err := u.ServeDNS(context.Background(), &plugintest.ResponseWriter{RemoteIP: "invalid"}, query); err != nil {
		t.Fatal(err)
	}
	if forwarded != query {
		t.Fatal("copied a request with an invalid client address")
	}
	if query.IsEdns0() != nil {
		t.Fatal("added EDNS to a request with an invalid client address")
	}
}

func TestServeDNSStripsSynthesizedOPTFromResponse(t *testing.T) {
	query := newQuery()
	response := new(dns.Msg)
	response.SetReply(query)
	response.SetEdns0(4096, false)
	response.IsEdns0().Option = []dns.EDNS0{
		&dns.EDNS0_LOCAL{Code: optionCode, Data: []byte{1}},
		&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "deadbeef"},
	}
	response.Extra = append(response.Extra, &dns.TXT{
		Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: 60},
		Txt: []string{"value"},
	})
	original := packed(t, response)
	recorder := dnstest.NewRecorder(&plugintest.ResponseWriter{RemoteIP: "2001:db8::1"})

	u := &Umbrella{
		Next: plugin.HandlerFunc(func(_ context.Context, writer dns.ResponseWriter, _ *dns.Msg) (int, error) {
			return dns.RcodeSuccess, writer.WriteMsg(response)
		}),
		option: newOptionTemplate(1, [8]byte{}),
	}

	if _, err := u.ServeDNS(context.Background(), recorder, query); err != nil {
		t.Fatal(err)
	}
	if recorder.Msg.IsEdns0() != nil {
		t.Fatal("response contains an OPT record")
	}
	if len(recorder.Msg.Extra) != 1 || recorder.Msg.Extra[0].Header().Rrtype != dns.TypeTXT {
		t.Fatalf("got extra records %v", recorder.Msg.Extra)
	}
	if !bytes.Equal(packed(t, response), original) {
		t.Fatal("mutated the downstream response")
	}
}

func TestServeDNSStripsOnlyUmbrellaOptionsFromEDNSResponse(t *testing.T) {
	query := newQuery()
	query.SetEdns0(1232, false)
	response := new(dns.Msg)
	response.SetReply(query)
	response.SetEdns0(1232, false)
	response.IsEdns0().Option = []dns.EDNS0{
		&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "deadbeef"},
		&dns.EDNS0_LOCAL{Code: optionCode, Data: []byte{1}},
		&dns.EDNS0_SUBNET{
			Code:          dns.EDNS0SUBNET,
			Family:        2,
			SourceNetmask: 56,
			SourceScope:   48,
			Address:       net.ParseIP("2001:db8:1234:5600::"),
		},
		&dns.EDNS0_LOCAL{Code: 65001, Data: []byte{2}},
		&dns.EDNS0_LOCAL{Code: optionCode, Data: []byte{3}},
	}
	original := packed(t, response)
	recorder := dnstest.NewRecorder(&plugintest.ResponseWriter{RemoteIP: "192.0.2.1"})

	u := &Umbrella{
		Next: plugin.HandlerFunc(func(_ context.Context, writer dns.ResponseWriter, _ *dns.Msg) (int, error) {
			return dns.RcodeSuccess, writer.WriteMsg(response)
		}),
		option: newOptionTemplate(1, [8]byte{}),
	}

	if _, err := u.ServeDNS(context.Background(), recorder, query); err != nil {
		t.Fatal(err)
	}
	opt := recorder.Msg.IsEdns0()
	if opt == nil {
		t.Fatal("response has no OPT record")
	}
	if got, want := optionCodes(opt), []uint16{dns.EDNS0NSID, dns.EDNS0SUBNET, 65001}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got option codes %v, want %v", got, want)
	}
	subnet, ok := opt.Option[1].(*dns.EDNS0_SUBNET)
	if !ok || subnet.Family != 2 || subnet.SourceNetmask != 56 || subnet.SourceScope != 48 || !subnet.Address.Equal(net.ParseIP("2001:db8:1234:5600::")) {
		t.Fatalf("changed response ECS option to %#v", opt.Option[1])
	}
	if !bytes.Equal(packed(t, response), original) {
		t.Fatal("mutated the downstream response")
	}
}

func TestServeDNSResponseFastPath(t *testing.T) {
	tests := []struct {
		name       string
		clientEDNS bool
		response   func(*dns.Msg) *dns.Msg
	}{
		{
			name: "no client or response EDNS",
			response: func(query *dns.Msg) *dns.Msg {
				return new(dns.Msg).SetReply(query)
			},
		},
		{
			name:       "unrelated response EDNS",
			clientEDNS: true,
			response: func(query *dns.Msg) *dns.Msg {
				response := new(dns.Msg).SetReply(query)
				response.SetEdns0(1232, false)
				response.IsEdns0().Option = append(response.IsEdns0().Option, &dns.EDNS0_NSID{Code: dns.EDNS0NSID})
				return response
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := newQuery()
			if test.clientEDNS {
				query.SetEdns0(1232, false)
			}
			response := test.response(query)
			recorder := dnstest.NewRecorder(&plugintest.ResponseWriter{RemoteIP: "192.0.2.1"})
			u := &Umbrella{
				Next: plugin.HandlerFunc(func(_ context.Context, writer dns.ResponseWriter, _ *dns.Msg) (int, error) {
					return dns.RcodeSuccess, writer.WriteMsg(response)
				}),
				option: newOptionTemplate(1, [8]byte{}),
			}

			if _, err := u.ServeDNS(context.Background(), recorder, query); err != nil {
				t.Fatal(err)
			}
			if recorder.Msg != response {
				t.Fatal("copied a response that did not need sanitizing")
			}
		})
	}
}

func newQuery() *dns.Msg {
	return new(dns.Msg).SetQuestion("example.org.", dns.TypeA)
}

func optionCodes(opt *dns.OPT) []uint16 {
	codes := make([]uint16, len(opt.Option))
	for i, option := range opt.Option {
		codes[i] = option.Option()
	}
	return codes
}

func packed(t *testing.T, message *dns.Msg) []byte {
	t.Helper()
	packed, err := message.Pack()
	if err != nil {
		t.Fatal(err)
	}
	return packed
}
