# coredns-umbrella

`coredns-umbrella` is an external CoreDNS plugin for forwarding IPv4 client
identity information to Cisco Umbrella. It adds Cisco's EDNS0 option `20292`
(`0x4F44`) immediately before a query reaches CoreDNS's `forward` plugin.

The initial compatibility target is CoreDNS 1.14.3.

## Status

The repository structure and protocol contract are defined, but the plugin is
not implemented yet.

## Corefile

```corefile
. {
    umbrella device_id 0123456789abcdef organization_id 12345678
    forward . 208.67.222.222 208.67.220.220
}
```

The directive is intentionally top-level. It must be used in a server block
whose downstream forwarding path is dedicated to Cisco Umbrella.

Requests whose client address is not IPv4 pass through unchanged.

## Planned wire format

The plugin will emit only Cisco's current option `20292`. It will not emit the
legacy device-only option `26946`.

The 28-byte option payload is:

| Bytes | Meaning |
| --- | --- |
| `4f 44 4e 53` | Magic value `ODNS` |
| `01` | Version |
| `00` | Flags |
| `00 08` + 4 bytes | Organization ID, unsigned and big-endian |
| `00 10` + 4 bytes | Client IPv4 address |
| `00 40` + 8 bytes | Device ID decoded from 16 hexadecimal characters |

## External plugin registration

CoreDNS plugins are statically linked. Add the following entry immediately
before `forward` in CoreDNS's `plugin.cfg`:

```text
umbrella:github.com/xdkr/coredns-umbrella
forward:forward
```

Then generate and build CoreDNS. The repository will include a pinned,
reproducible build wrapper following the approach used by
[`dokku/coredns-docker`](https://github.com/dokku/coredns-docker).

## License

MIT
