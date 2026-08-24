package umbrella

import (
	"encoding/binary"

	"github.com/miekg/dns"
)

const optionCode uint16 = 20292

type optionTemplate [28]byte

func newOptionTemplate(organizationID uint32, deviceID [8]byte) optionTemplate {
	data := optionTemplate{
		'O', 'D', 'N', 'S', 0x01, 0x00,
		0x00, 0x08, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x10, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x40, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	}

	binary.BigEndian.PutUint32(data[8:12], organizationID)
	copy(data[20:28], deviceID[:])

	return data
}

func (data optionTemplate) option(ipv4 [4]byte) *dns.EDNS0_LOCAL {
	copy(data[14:18], ipv4[:])
	return &dns.EDNS0_LOCAL{Code: optionCode, Data: data[:]}
}
