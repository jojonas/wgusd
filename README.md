# wgusd

WGuSD (Wireguard &micro; Service Discovery) is inspired by Jordan Whited's [wgsd](https://github.com/jwhited/wgsd) in that it implements a DNS service discovery scheme for Wireguard interfaces.

Reasons why I (re)implemented wgsd:
 * I needed a standalone client, not a plugin for CoreDNS.
 * I don't want to reveal the endpoint's public key.
 * The DNS records must be as simple as possible as they are created manually.

Limitations, compared with wgsd:
 * A zone can only host a single peer. Only Wireguard configurations with exactly one peer are supported (client/server, hub-and-spoke).
 * DNS records must be created manually.

## Building and Installing

`wgusd` can be build in the same way as any other Golang application:

```bash
git clone https://github.com/jojonas/wgusd
cd wgusd
go build .
```

The `dist/` folder also contains a Systemd `.service` and `.timer` file which can be used to call wgusd in regular intervals.
The file `dist/nfpm.yaml` contains a definition for the great Go packaging tool [nfpm](https://nfpm.goreleaser.com/). The following commands can be used to build a debian package:
```bash
go build .
cd dist/
nfpm package -r deb
```

## Configuration and Running

`wgusd` pulls its configuration from the command line. The options can be viewed with `wgusd --help`:

```text
$ ./wgusd --help
Usage of ./wgusd:
      --fallback string    Fallback endpoint, configured when lookup fails
  -i, --interface string   Wireguard interface to (re)configure
  -v, --verbose count      Verbose output (use multiple times to get debug output)
  -z, --zone string        Zone to query for SRV records
```

Default values are pulled from the environment (variables `WGUSD_ZONE`, `WGUSD_INTERFACE`, `WGUSD_FALLBACK`). This is for example used by the Systemd service unit which sources environment variables from `/etc/default/wgusd`.

## Required DNS Records

In order for `wgusd` to reconfigure the specified interface, it requests a DNS SRV record from the specified zone (scheme: `_wireguard._udp.<zone>`). The record should be set as follows:
```text
<priority> <weight> <port> <hostname>
```

For example:

```text
10 10 51820 my-wireguard-server.example.com.
```

The hostname is then resolved as a "normal" `A`/`AAAA` record. Priority and weight _must_ be set for a valid SRV record, but do not play any role in this scenario. If multiple records are returned, `wgusd` chooses the one with the lowest priority number and, if priorities are tied, with the largest weight.

## Legal

`wgusd` is licensed under the MIT license (see `LICENSE.txt`).

WireGuard is a registered trademark of Jason A. Donenfeld.