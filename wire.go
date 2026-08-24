package umbrella

import (
	"encoding/binary"
	"net/netip"

	"github.com/miekg/dns"
)

const (
	optionCode               uint16 = 20292
	organizationFieldType    uint16 = 0x0008
	ipv4AddressFieldType     uint16 = 0x0010
	ipv6AddressFieldType     uint16 = 0x0020
	deviceIDFieldType        uint16 = 0x0040
	optionHeaderLength              = 6
	organizationFieldLength         = 6
	addressFieldHeaderLength        = 2
	deviceIDFieldLength             = 10
)

type optionTemplate struct {
	organizationID uint32
	deviceID       [8]byte
}

func newOptionTemplate(organizationID uint32, deviceID [8]byte) optionTemplate {
	return optionTemplate{organizationID: organizationID, deviceID: deviceID}
}

func (template optionTemplate) option(address netip.Addr) *dns.EDNS0_LOCAL {
	address = address.Unmap().WithZone("")

	var addressType uint16
	var addressData []byte
	if address.Is4() {
		addressType = ipv4AddressFieldType
		ipv4 := address.As4()
		addressData = ipv4[:]
	} else {
		addressType = ipv6AddressFieldType
		ipv6 := address.As16()
		addressData = ipv6[:]
	}

	data := make([]byte, optionHeaderLength+organizationFieldLength+addressFieldHeaderLength+len(addressData)+deviceIDFieldLength)
	copy(data[0:4], "ODNS")
	data[4] = 0x01
	data[5] = 0x00

	binary.BigEndian.PutUint16(data[6:8], organizationFieldType)
	binary.BigEndian.PutUint32(data[8:12], template.organizationID)
	binary.BigEndian.PutUint16(data[12:14], addressType)
	copy(data[14:], addressData)

	deviceOffset := 14 + len(addressData)
	binary.BigEndian.PutUint16(data[deviceOffset:deviceOffset+2], deviceIDFieldType)
	copy(data[deviceOffset+2:], template.deviceID[:])

	return &dns.EDNS0_LOCAL{Code: optionCode, Data: data}
}
