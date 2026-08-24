# coredns-umbrella

[![Lines of code](https://img.shields.io/endpoint?url=https%3A%2F%2Fghloc.vercel.app%2Fapi%2Fxdkr%2Fcoredns-umbrella%2Fbadge)](https://ghloc.dev/xdkr/coredns-umbrella?branch=main) [![Tests](https://img.shields.io/github/actions/workflow/status/xdkr/coredns-umbrella/ci.yml?branch=main&event=push&label=tests)](https://github.com/xdkr/coredns-umbrella/actions/workflows/ci.yml) [![Release](https://img.shields.io/github/v/release/xdkr/coredns-umbrella?sort=semver&label=release)](https://github.com/xdkr/coredns-umbrella/releases/latest)

CoreDNS-Umbrella is a CoreDNS plugin for forwarding client identity information
to Cisco Umbrella. It adds Cisco's EDNS0 options to a query.

## Corefile

```corefile
. {
    umbrella organization_id 12345678 device_id 0123456789abcdef
    forward . 208.67.222.222 208.67.220.220
}
```

## Wire format

https://developer.cisco.com/docs/cloud-security/network-devices-with-cisco-umbrella-dns/#identify-dns-traffic

The option payload is 28 bytes for an IPv4 client and 40 bytes for an IPv6
client:

| Bytes | Meaning |
| --- | --- |
| `4f 44 4e 53` | Magic value `ODNS` |
| `01` | Version |
| `00` | Flags |
| `00 08` + 4 bytes | Organization ID, unsigned and big-endian |
| `00 10` + 4 bytes | Client IPv4 address, for an IPv4 client |
| `00 20` + 16 bytes | Client IPv6 address, for an IPv6 client |
| `00 40` + 8 bytes | 8-byte device ID |

Each payload contains exactly one client address field matching the connecting
client's address family.

## External plugin registration

CoreDNS plugins are statically linked. Add the following entry immediately
before `forward` in CoreDNS's `plugin.cfg`:

```text
umbrella:github.com/xdkr/coredns-umbrella
forward:forward
```

Build locally:

```sh
make build
```

## License

MIT
