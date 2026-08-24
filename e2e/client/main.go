package main

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/miekg/dns"
)

const umbrellaOptionCode uint16 = 20292

const (
	requestCookie  = "0102030405060708"
	responseCookie = "0102030405060708a1a2a3a4a5a6a7a8"
	responseNSID   = "66616b652d636973636f"
)

type testCase struct {
	name     string
	network  string
	withEDNS bool
}

func main() {
	server := envOrDefault("DNS_SERVER", "172.30.53.20:1053")
	if err := waitForServer(server, 20*time.Second); err != nil {
		log.Fatal(err)
	}

	tests := []testCase{
		{name: "udp-no-edns.e2e.test.", network: "udp"},
		{name: "udp-with-edns.e2e.test.", network: "udp", withEDNS: true},
		{name: "tcp-no-edns.e2e.test.", network: "tcp"},
		{name: "tcp-with-edns.e2e.test.", network: "tcp", withEDNS: true},
	}

	for _, test := range tests {
		if err := runTest(server, test); err != nil {
			log.Fatalf("FAIL %s: %v", test.name, err)
		}
		log.Printf("PASS %s via %s", test.name, test.network)
	}

	log.Printf("PASS all %d end-to-end cases", len(tests))
}

func waitForServer(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("CoreDNS at %s did not become ready within %s", address, timeout)
}

func runTest(server string, test testCase) error {
	query := new(dns.Msg)
	query.SetQuestion(test.name, dns.TypeA)
	if test.withEDNS {
		query.SetEdns0(1232, true)
		query.IsEdns0().Option = []dns.EDNS0{
			&dns.EDNS0_NSID{Code: dns.EDNS0NSID},
			requestSubnet(test.network),
			&dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: requestCookie},
			&dns.EDNS0_LOCAL{Code: 65001, Data: []byte{1, 2, 3}},
			&dns.EDNS0_LOCAL{Code: umbrellaOptionCode, Data: []byte("forged-client-identity")},
		}
	}

	client := &dns.Client{Net: test.network, Timeout: 5 * time.Second}
	response, _, err := client.Exchange(query, server)
	if err != nil {
		return fmt.Errorf("exchange: %w", err)
	}
	if response.Rcode != dns.RcodeSuccess {
		return fmt.Errorf("rcode is %s", dns.RcodeToString[response.Rcode])
	}
	if len(response.Answer) != 1 {
		return fmt.Errorf("got %d answers, want 1", len(response.Answer))
	}
	answer, ok := response.Answer[0].(*dns.A)
	if !ok || !answer.A.Equal(net.IPv4(203, 0, 113, 42)) {
		return fmt.Errorf("unexpected answer %v", response.Answer[0])
	}

	opt := response.IsEdns0()
	if !test.withEDNS {
		if opt != nil {
			return errors.New("response retained the synthesized OPT record")
		}
		return nil
	}
	if opt == nil {
		return errors.New("response lost the client-requested OPT record")
	}
	if opt.UDPSize() != 1232 || !opt.Do() {
		return fmt.Errorf("response EDNS has size=%d DO=%t", opt.UDPSize(), opt.Do())
	}
	wantCodes := []uint16{dns.EDNS0NSID, dns.EDNS0SUBNET, dns.EDNS0COOKIE, 65002}
	if got := optionCodes(opt); !equalCodes(got, wantCodes) {
		return fmt.Errorf("response option codes are %v, want %v", got, wantCodes)
	}
	if err := validateResponseOptions(opt, test.network); err != nil {
		return err
	}

	return nil
}

func requestSubnet(network string) *dns.EDNS0_SUBNET {
	if network == "tcp" {
		return &dns.EDNS0_SUBNET{
			Code:          dns.EDNS0SUBNET,
			Family:        2,
			SourceNetmask: 56,
			Address:       net.ParseIP("2001:db8:1234:5600::"),
		}
	}
	return &dns.EDNS0_SUBNET{
		Code:          dns.EDNS0SUBNET,
		Family:        1,
		SourceNetmask: 24,
		Address:       net.IPv4(198, 51, 100, 0),
	}
}

func validateResponseOptions(opt *dns.OPT, network string) error {
	nsid, ok := opt.Option[0].(*dns.EDNS0_NSID)
	if !ok || nsid.Nsid != responseNSID {
		return fmt.Errorf("response NSID is %#v, want %q", opt.Option[0], responseNSID)
	}

	if err := validateResponseSubnet(opt.Option[1], network); err != nil {
		return err
	}

	cookie, ok := opt.Option[2].(*dns.EDNS0_COOKIE)
	if !ok || cookie.Cookie != responseCookie {
		return fmt.Errorf("response cookie is %#v, want %q", opt.Option[2], responseCookie)
	}

	local, ok := opt.Option[3].(*dns.EDNS0_LOCAL)
	if !ok || local.Code != 65002 || !bytes.Equal(local.Data, []byte{4, 5, 6}) {
		return fmt.Errorf("response private option is %#v", opt.Option[3])
	}

	return nil
}

func validateResponseSubnet(option dns.EDNS0, network string) error {
	subnet, ok := option.(*dns.EDNS0_SUBNET)
	if !ok {
		return fmt.Errorf("response ECS has type %T", option)
	}

	want := requestSubnet(network)
	want.SourceScope = want.SourceNetmask
	if subnet.Family != want.Family ||
		subnet.SourceNetmask != want.SourceNetmask ||
		subnet.SourceScope != want.SourceScope ||
		!subnet.Address.Equal(want.Address) {
		return fmt.Errorf(
			"response ECS is family=%d source=%d scope=%d address=%s, want family=%d source=%d scope=%d address=%s",
			subnet.Family,
			subnet.SourceNetmask,
			subnet.SourceScope,
			subnet.Address,
			want.Family,
			want.SourceNetmask,
			want.SourceScope,
			want.Address,
		)
	}
	return nil
}

func optionCodes(opt *dns.OPT) []uint16 {
	codes := make([]uint16, len(opt.Option))
	for i, option := range opt.Option {
		codes[i] = option.Option()
	}
	return codes
}

func equalCodes(left, right []uint16) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
