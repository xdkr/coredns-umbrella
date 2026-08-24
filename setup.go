package umbrella

import (
	"encoding/hex"
	"errors"
	"strconv"

	"github.com/coredns/caddy"
	"github.com/coredns/coredns/core/dnsserver"
	"github.com/coredns/coredns/plugin"
)

const pluginName = "umbrella"

var (
	errArguments      = errors.New("expected device_id and organization_id")
	errBlock          = errors.New("block syntax is not supported")
	errDeviceID       = errors.New("device_id must be 16 hexadecimal characters")
	errOrganizationID = errors.New("organization_id must be an unsigned 32-bit decimal integer")
)

func init() { plugin.Register(pluginName, setup) }

func setup(c *caddy.Controller) error {
	option, err := parse(c)
	if err != nil {
		return plugin.Error(pluginName, err)
	}

	dnsserver.GetConfig(c).AddPlugin(func(next plugin.Handler) plugin.Handler {
		return &Umbrella{Next: next, option: option}
	})

	return nil
}

func parse(c *caddy.Controller) (optionTemplate, error) {
	var parsed bool
	var option optionTemplate

	for c.Next() {
		if parsed {
			return optionTemplate{}, plugin.ErrOnce
		}
		parsed = true

		args := c.RemainingArgs()
		if len(args) != 4 {
			return optionTemplate{}, errArguments
		}

		var deviceID [8]byte
		var organizationID uint32
		var hasDeviceID bool
		var hasOrganizationID bool

		for i := 0; i < len(args); i += 2 {
			switch args[i] {
			case "device_id":
				if hasDeviceID {
					return optionTemplate{}, errors.New("device_id may only be specified once")
				}
				var err error
				deviceID, err = parseDeviceID(args[i+1])
				if err != nil {
					return optionTemplate{}, err
				}
				hasDeviceID = true

			case "organization_id":
				if hasOrganizationID {
					return optionTemplate{}, errors.New("organization_id may only be specified once")
				}
				var err error
				organizationID, err = parseOrganizationID(args[i+1])
				if err != nil {
					return optionTemplate{}, err
				}
				hasOrganizationID = true

			default:
				return optionTemplate{}, errors.New("unknown property")
			}
		}

		if c.NextBlock() {
			return optionTemplate{}, errBlock
		}
		if !hasDeviceID || !hasOrganizationID {
			return optionTemplate{}, errArguments
		}

		option = newOptionTemplate(organizationID, deviceID)
	}

	if !parsed {
		return optionTemplate{}, errArguments
	}

	return option, nil
}

func parseDeviceID(value string) ([8]byte, error) {
	var deviceID [8]byte
	if len(value) != hex.EncodedLen(len(deviceID)) {
		return deviceID, errDeviceID
	}

	decoded, err := hex.DecodeString(value)
	if err != nil {
		return deviceID, errDeviceID
	}
	copy(deviceID[:], decoded)

	return deviceID, nil
}

func parseOrganizationID(value string) (uint32, error) {
	if value == "" {
		return 0, errOrganizationID
	}
	for i := range value {
		if value[i] < '0' || value[i] > '9' {
			return 0, errOrganizationID
		}
	}

	parsed, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, errOrganizationID
	}

	return uint32(parsed), nil
}
