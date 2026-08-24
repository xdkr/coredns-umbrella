package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/miekg/dns"
)

const umbrellaOptionCode uint16 = 20292

const (
	requestCookie  = "0102030405060708"
	responseCookie = "0102030405060708a1a2a3a4a5a6a7a8"
	responseNSID   = "66616b652d636973636f"
)

type validator struct {
	organizationID uint32
	clientIP       netip.Addr
	deviceID       [8]byte
}

func main() {
	listenAddress := envOrDefault("LISTEN_ADDRESS", ":5300")
	validator, err := validatorFromEnvironment()
	if err != nil {
		log.Fatal(err)
	}

	handler := dns.HandlerFunc(validator.serveDNS)
	errors := make(chan error, 2)
	for _, network := range []string{"udp", "tcp"} {
		server := &dns.Server{Addr: listenAddress, Net: network, Handler: handler}
		go func() {
			log.Printf("fake Cisco receiver listening on %s/%s", listenAddress, server.Net)
			errors <- server.ListenAndServe()
		}()
	}

	log.Fatal(<-errors)
}

func validatorFromEnvironment() (*validator, error) {
	organizationValue := os.Getenv("EXPECTED_ORGANIZATION_ID")
	organizationID, err := strconv.ParseUint(organizationValue, 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse EXPECTED_ORGANIZATION_ID: %w", err)
	}

	clientAddress, err := netip.ParseAddr(os.Getenv("EXPECTED_CLIENT_IP"))
	if err != nil {
		return nil, errors.New("EXPECTED_CLIENT_IP must be an IP address")
	}
	clientAddress = clientAddress.Unmap().WithZone("")

	deviceValue := os.Getenv("EXPECTED_DEVICE_ID")
	decodedDevice, err := hex.DecodeString(deviceValue)
	if err != nil || len(decodedDevice) != 8 {
		return nil, errors.New("EXPECTED_DEVICE_ID must be 16 hexadecimal characters")
	}

	validator := &validator{
		organizationID: uint32(organizationID),
		clientIP:       clientAddress,
	}
	copy(validator.deviceID[:], decodedDevice)

	return validator, nil
}

func (v *validator) serveDNS(writer dns.ResponseWriter, request *dns.Msg) {
	network := writer.RemoteAddr().Network()
	data, err := v.validateRequest(request, network)
	if err != nil {
		log.Printf("FAIL %s from %s: %v", questionName(request), writer.RemoteAddr(), err)
		response := new(dns.Msg)
		response.SetRcode(request, dns.RcodeServerFailure)
		_ = writer.WriteMsg(response)
		return
	}

	log.Printf(
		"PASS %s via %s: organization=%d client=%s device=%x unrelated-edns=preserved",
		questionName(request),
		network,
		v.organizationID,
		v.clientIP,
		v.deviceID,
	)

	response := new(dns.Msg).SetReply(request)
	response.Answer = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{
			Name:   request.Question[0].Name,
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		A: net.IPv4(203, 0, 113, 42),
	}}
	response.SetEdns0(4096, false)
	response.IsEdns0().Option = []dns.EDNS0{
		&dns.EDNS0_NSID{Code: dns.EDNS0NSID, Nsid: responseNSID},
		responseSubnet(network),
		&dns.EDNS0_COOKIE{Code: dns.EDNS0COOKIE, Cookie: responseCookie},
		&dns.EDNS0_LOCAL{Code: 65002, Data: []byte{4, 5, 6}},
		&dns.EDNS0_LOCAL{Code: umbrellaOptionCode, Data: append([]byte(nil), data...)},
	}

	if err := writer.WriteMsg(response); err != nil {
		log.Printf("write response: %v", err)
	}
}

func (v *validator) validateRequest(request *dns.Msg, network string) ([]byte, error) {
	if len(request.Question) != 1 {
		return nil, fmt.Errorf("got %d questions, want 1", len(request.Question))
	}

	optCount := 0
	umbrellaCount := 0
	var umbrellaData []byte
	for _, record := range request.Extra {
		opt, ok := record.(*dns.OPT)
		if !ok {
			continue
		}
		optCount++
		for _, option := range opt.Option {
			if option.Option() != umbrellaOptionCode {
				continue
			}
			local, ok := option.(*dns.EDNS0_LOCAL)
			if !ok {
				return nil, fmt.Errorf("Umbrella option has type %T", option)
			}
			umbrellaCount++
			umbrellaData = local.Data
		}
	}
	if optCount != 1 {
		return nil, fmt.Errorf("got %d OPT records, want 1", optCount)
	}
	if umbrellaCount != 1 {
		return nil, fmt.Errorf("got %d Umbrella options, want 1", umbrellaCount)
	}

	if err := v.validatePayload(umbrellaData); err != nil {
		return nil, err
	}
	if err := validateEDNSProperties(request, network); err != nil {
		return nil, err
	}

	return umbrellaData, nil
}

func (v *validator) validatePayload(data []byte) error {
	clientData := v.clientIP.AsSlice()
	addressType := uint16(0x0020)
	if v.clientIP.Is4() {
		addressType = 0x0010
	}
	wantLength := 24 + len(clientData)
	if len(data) != wantLength {
		return fmt.Errorf("payload is %d bytes, want %d", len(data), wantLength)
	}
	if !bytes.Equal(data[0:4], []byte("ODNS")) {
		return fmt.Errorf("bad magic %x", data[0:4])
	}
	if data[4] != 1 || data[5] != 0 {
		return fmt.Errorf("bad version or flags %x", data[4:6])
	}
	if got := binary.BigEndian.Uint16(data[6:8]); got != 0x0008 {
		return fmt.Errorf("organization field type is %#x", got)
	}
	if got := binary.BigEndian.Uint32(data[8:12]); got != v.organizationID {
		return fmt.Errorf("organization is %d, want %d", got, v.organizationID)
	}
	if got := binary.BigEndian.Uint16(data[12:14]); got != addressType {
		return fmt.Errorf("client address field type is %#x, want %#x", got, addressType)
	}
	addressEnd := 14 + len(clientData)
	if !bytes.Equal(data[14:addressEnd], clientData) {
		return fmt.Errorf("client address is %s, want %s", net.IP(data[14:addressEnd]), v.clientIP)
	}
	if got := binary.BigEndian.Uint16(data[addressEnd : addressEnd+2]); got != 0x0040 {
		return fmt.Errorf("device field type is %#x", got)
	}
	if !bytes.Equal(data[addressEnd+2:], v.deviceID[:]) {
		return fmt.Errorf("device is %x, want %x", data[addressEnd+2:], v.deviceID)
	}

	return nil
}

func validateEDNSProperties(request *dns.Msg, network string) error {
	opt := request.IsEdns0()
	withEDNS := strings.Contains(questionName(request), "with-edns")
	if withEDNS {
		if opt.UDPSize() != 1232 || !opt.Do() {
			return fmt.Errorf("existing EDNS changed to size=%d DO=%t", opt.UDPSize(), opt.Do())
		}
		if got, want := optionCodes(opt), []uint16{dns.EDNS0NSID, dns.EDNS0SUBNET, dns.EDNS0COOKIE, 65001, umbrellaOptionCode}; !equalCodes(got, want) {
			return fmt.Errorf("option codes are %v, want %v", got, want)
		}
		if err := validateRequestOptions(opt, network); err != nil {
			return err
		}
		return nil
	}

	if opt.UDPSize() != dns.MinMsgSize || opt.Do() {
		return fmt.Errorf("synthesized EDNS has size=%d DO=%t", opt.UDPSize(), opt.Do())
	}
	if got, want := optionCodes(opt), []uint16{umbrellaOptionCode}; !equalCodes(got, want) {
		return fmt.Errorf("option codes are %v, want %v", got, want)
	}

	return nil
}

func validateRequestOptions(opt *dns.OPT, network string) error {
	nsid, ok := opt.Option[0].(*dns.EDNS0_NSID)
	if !ok || nsid.Nsid != "" {
		return fmt.Errorf("request NSID is %#v", opt.Option[0])
	}

	if err := validateRequestSubnet(opt.Option[1], network); err != nil {
		return err
	}

	cookie, ok := opt.Option[2].(*dns.EDNS0_COOKIE)
	if !ok || cookie.Cookie != requestCookie {
		return fmt.Errorf("request cookie is %#v, want %q", opt.Option[2], requestCookie)
	}

	local, ok := opt.Option[3].(*dns.EDNS0_LOCAL)
	if !ok || local.Code != 65001 || !bytes.Equal(local.Data, []byte{1, 2, 3}) {
		return fmt.Errorf("request private option is %#v", opt.Option[3])
	}

	return nil
}

func validateRequestSubnet(option dns.EDNS0, network string) error {
	subnet, ok := option.(*dns.EDNS0_SUBNET)
	if !ok {
		return fmt.Errorf("request ECS has type %T", option)
	}

	want := requestSubnet(network)
	if subnet.Family != want.Family ||
		subnet.SourceNetmask != want.SourceNetmask ||
		subnet.SourceScope != 0 ||
		!subnet.Address.Equal(want.Address) {
		return fmt.Errorf(
			"request ECS is family=%d source=%d scope=%d address=%s, want family=%d source=%d scope=0 address=%s",
			subnet.Family,
			subnet.SourceNetmask,
			subnet.SourceScope,
			subnet.Address,
			want.Family,
			want.SourceNetmask,
			want.Address,
		)
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

func responseSubnet(network string) *dns.EDNS0_SUBNET {
	subnet := requestSubnet(network)
	subnet.SourceScope = subnet.SourceNetmask
	return subnet
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

func questionName(message *dns.Msg) string {
	if len(message.Question) == 0 {
		return "<no-question>"
	}
	return message.Question[0].Name
}

func envOrDefault(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
