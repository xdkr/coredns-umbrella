package umbrella

import (
	"bytes"
	"context"
	"net"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/forward"
	"github.com/coredns/coredns/plugin/pkg/dnstest"
	"github.com/coredns/coredns/plugin/pkg/proxy"
	"github.com/coredns/coredns/plugin/pkg/transport"
	plugintest "github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

func TestForwardIntegration(t *testing.T) {
	tests := []struct {
		name             string
		clientEDNS       bool
		wantRequestCodes []uint16
	}{
		{"without client EDNS", false, []uint16{optionCode}},
		{"with client EDNS", true, []uint16{dns.EDNS0NSID, optionCode}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			upstreamAddress, requests := startUDPUpstream(t)
			forwarder := newForwarder(t, upstreamAddress)
			handler := configuredHandler(t, forwarder,
				"umbrella device_id 0123456789abcdef organization_id 012345678")

			query := newQuery()
			if test.clientEDNS {
				query.SetEdns0(1232, true)
				query.IsEdns0().Option = append(query.IsEdns0().Option, &dns.EDNS0_NSID{Code: dns.EDNS0NSID})
			}
			original := packed(t, query)

			recorder := dnstest.NewRecorder(&plugintest.ResponseWriter{RemoteIP: "192.168.1.55"})
			writer := request.NewScrubWriter(query, recorder)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()

			rcode, err := handler.ServeDNS(ctx, writer, query)
			if err != nil {
				t.Fatal(err)
			}
			if rcode != dns.RcodeSuccess {
				t.Fatalf("got rcode %d, want %d", rcode, dns.RcodeSuccess)
			}
			if !bytes.Equal(packed(t, query), original) {
				t.Fatal("mutated the client request")
			}

			var forwarded *dns.Msg
			select {
			case forwarded = <-requests:
			case <-ctx.Done():
				t.Fatal("upstream did not receive the query")
			}

			opt := forwarded.IsEdns0()
			if opt == nil {
				t.Fatal("upstream query has no OPT record")
			}
			if got := optionCodes(opt); !reflect.DeepEqual(got, test.wantRequestCodes) {
				t.Fatalf("got upstream option codes %v, want %v", got, test.wantRequestCodes)
			}
			if test.clientEDNS {
				if opt.UDPSize() != 1232 || !opt.Do() {
					t.Fatalf("got upstream UDP size %d and DO %t", opt.UDPSize(), opt.Do())
				}
			} else if opt.UDPSize() != dns.DefaultMsgSize || opt.Do() {
				t.Fatalf("got upstream UDP size %d and DO %t", opt.UDPSize(), opt.Do())
			}

			local, ok := opt.Option[len(opt.Option)-1].(*dns.EDNS0_LOCAL)
			if !ok {
				t.Fatalf("got option type %T, want *dns.EDNS0_LOCAL", opt.Option[len(opt.Option)-1])
			}
			wantData := []byte{
				0x4f, 0x44, 0x4e, 0x53, 0x01, 0x00,
				0x00, 0x08, 0x00, 0xbc, 0x61, 0x4e,
				0x00, 0x10, 0xc0, 0xa8, 0x01, 0x37,
				0x00, 0x40, 0x01, 0x23, 0x45, 0x67,
				0x89, 0xab, 0xcd, 0xef,
			}
			if local.Code != optionCode || !bytes.Equal(local.Data, wantData) {
				t.Fatalf("got upstream option %d:%x", local.Code, local.Data)
			}

			if recorder.Msg == nil || len(recorder.Msg.Answer) != 1 {
				t.Fatalf("got client response %v", recorder.Msg)
			}
			responseOPT := recorder.Msg.IsEdns0()
			if !test.clientEDNS {
				if responseOPT != nil {
					t.Fatal("client response contains an OPT record")
				}
				return
			}
			if responseOPT == nil {
				t.Fatal("client response has no OPT record")
			}
			if responseOPT.UDPSize() != 1232 || !responseOPT.Do() {
				t.Fatalf("got response UDP size %d and DO %t", responseOPT.UDPSize(), responseOPT.Do())
			}
			if got, want := optionCodes(responseOPT), []uint16{dns.EDNS0NSID}; !reflect.DeepEqual(got, want) {
				t.Fatalf("got response option codes %v, want %v", got, want)
			}
		})
	}
}

func startUDPUpstream(t *testing.T) (string, <-chan *dns.Msg) {
	t.Helper()

	connection, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	requests := make(chan *dns.Msg, 1)
	server := &dns.Server{
		PacketConn: connection,
		Handler: dns.HandlerFunc(func(writer dns.ResponseWriter, query *dns.Msg) {
			if len(query.Question) == 1 && query.Question[0].Name == "example.org." {
				select {
				case requests <- query.Copy():
				default:
				}
			}

			response := new(dns.Msg).SetReply(query)
			response.Answer = append(response.Answer, &dns.A{
				Hdr: dns.RR_Header{Name: "example.org.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
				A:   net.IPv4(198, 51, 100, 42),
			})
			response.SetEdns0(dns.DefaultMsgSize, false)
			response.IsEdns0().Option = []dns.EDNS0{
				&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: "deadbeef"},
				&dns.EDNS0_LOCAL{Code: optionCode, Data: []byte{1, 2, 3}},
			}
			_ = writer.WriteMsg(response)
		}),
	}
	done := make(chan error, 1)
	go func() {
		done <- server.ActivateAndServe()
	}()

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.ShutdownContext(ctx); err != nil {
			t.Errorf("shut down upstream: %v", err)
		}
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("serve upstream: %v", err)
			}
		case <-ctx.Done():
			t.Error("timed out waiting for upstream shutdown")
		}
	})

	return connection.LocalAddr().String(), requests
}

func newForwarder(t *testing.T, address string) *forward.Forward {
	t.Helper()

	forwarder := forward.New()
	upstream := proxy.NewProxy("forward", address, transport.DNS)
	forwarder.SetProxy(upstream)

	t.Cleanup(func() {
		runtime.SetFinalizer(upstream, nil)
		upstream.Stop()
		upstream.GetTransport().Stop()
	})

	return forwarder
}

func configuredHandler(t *testing.T, next plugin.Handler, corefile string) *Umbrella {
	t.Helper()

	controller := caddy.NewTestController("dns", corefile)
	if err := setup(controller); err != nil {
		t.Fatal(err)
	}

	plugins := dnsserver.GetConfig(controller).Plugin
	if len(plugins) != 1 {
		t.Fatalf("got %d plugins, want 1", len(plugins))
	}
	handler, ok := plugins[0](next).(*Umbrella)
	if !ok {
		t.Fatalf("got handler type %T, want *Umbrella", plugins[0](next))
	}

	return handler
}
