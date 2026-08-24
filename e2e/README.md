# Docker end-to-end test

This harness has separate IPv4 and IPv6 profiles. Each profile runs three
containers on an isolated Docker network:

1. `client` sends DNS queries over UDP and TCP, both with and without EDNS.
2. `coredns` runs a CoreDNS binary compiled with the local Umbrella plugin.
3. `fake-cisco` decodes and validates the complete Umbrella EDNS payload before
   returning a deterministic DNS answer.

The receiver verifies the organization ID, device ID, original client address,
address-family-specific wire field and payload length, EDNS flags, option
replacement, and preservation of unrelated options. Those options include IPv4
and IPv6 ECS, DNS cookies, NSID, and a separate private-use option. The client
validates those options again on the response and ensures only the Umbrella
option is removed.

Run from the repository root:

```sh
make test-e2e
```

Run one address family independently with:

```sh
make test-e2e-ipv4
make test-e2e-ipv6
```

The IPv4 profile uses fixed addresses in `172.30.53.0/24`. The IPv6 profile
uses fixed addresses in `fd00:53:53::/64` and sends both client and upstream DNS
traffic over IPv6. This lets each receiver independently validate the client
identity encoded by the plugin.
