//go:build live

package umbrella

import (
	"context"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/coredns/coredns/plugin/pkg/dnstest"
	plugintest "github.com/coredns/coredns/plugin/test"
	"github.com/coredns/coredns/request"
	"github.com/miekg/dns"
)

func TestLiveUmbrella(t *testing.T) {
	deviceID := os.Getenv("UMBRELLA_DEVICE_ID")
	organizationID := os.Getenv("UMBRELLA_ORGANIZATION_ID")
	parsedOrganizationID, err := parseOrganizationID(organizationID)
	if _, deviceErr := parseDeviceID(deviceID); deviceErr != nil || err != nil {
		t.Fatal("invalid Umbrella validation credentials")
	}

	want := map[string]struct{}{
		"device " + deviceID: {},
		"organization id " + strconv.FormatUint(uint64(parsedOrganizationID), 10): {},
		"remoteip 192.168.1.55": {},
	}

	for _, resolver := range []string{"208.67.222.222", "208.67.220.220"} {
		t.Run(resolver, func(t *testing.T) {
			forwarder := newForwarder(t, net.JoinHostPort(resolver, "53"))
			handler := configuredHandler(t, forwarder,
				"umbrella device_id "+deviceID+" organization_id "+organizationID)

			query := new(dns.Msg)
			query.SetQuestion("debug.opendns.com.", dns.TypeTXT)
			recorder := dnstest.NewRecorder(&plugintest.ResponseWriter{RemoteIP: "192.168.1.55"})
			writer := request.NewScrubWriter(query, recorder)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			rcode, err := handler.ServeDNS(ctx, writer, query)
			if err != nil || rcode != dns.RcodeSuccess || recorder.Msg == nil {
				t.Fatal("Umbrella diagnostic query failed")
			}

			got := make(map[string]struct{}, len(want))
			for _, answer := range recorder.Msg.Answer {
				txt, ok := answer.(*dns.TXT)
				if !ok {
					continue
				}
				for _, value := range txt.Txt {
					if _, ok := want[value]; ok {
						got[value] = struct{}{}
					}
				}
			}
			for value := range want {
				if _, ok := got[value]; !ok {
					t.Fatal("Umbrella diagnostic response did not contain the injected identity")
				}
			}
		})
	}
}
