# net
This is a port of Go's "net" package.  The port offers a subset of Go's "net"
package.  The subset maintains Go 1 compatiblity guarantee.

The "net" package is modified to use netdev, TinyGo's network device driver interface.
Netdev replaces the OS syscall interface for I/O access to the networking
device.  See drivers/netdev for more information on netdev.

#### Table of Contents

- [Using "net" and "net/http" Packages](#using-net-and-nethttp-packages)
- ["net" Package](#net-package)
- [Maintaining "net"](#maintaining-net)

## Using "net" and "net/http" Packages

See README-net.md in drivers repo to more details on using "net" and "net/http"
packages in a TinyGo application.

## "net" Package

The "net" package is ported from Go 1.21.4.  The tree listings below shows the
files copied.  If the file is marked with an '\*', it is copied _and_ modified
to work with netdev.  If the file is marked with an '+', the file is new.  If
there is no mark, it is a straight copy.

```
src/net
├── dial.go			*
├── http
│   ├── httptest
│   │   ├── httptest.go		*
│   │   ├── recorder.go
│   │   └── server.go		*
│   ├── client.go		*
│   ├── clone.go
│   ├── cookie.go
│   ├── fs.go
│   ├── header.go		*
│   ├── http.go
│   ├── internal
│   │   ├── ascii
│   │   │   ├── print.go
│   │   │   └── print_test.go
│   │   ├── chunked.go
│   │   └── chunked_test.go
│   ├── jar.go
│   ├── method.go
│   ├── request.go		*
│   ├── response.go		*
│   ├── server.go		*
│   ├── sniff.go
│   ├── status.go
│   ├── transfer.go		*
│   └── transport.go		*
├── interface.go		*
├── ip.go
├── iprawsock.go		*
├── ipsock.go			*
├── lookup.go			*
├── mac.go
├── mac_test.go
├── netdev.go			+
├── net.go			*
├── parse.go
├── pipe.go
├── README.md
├── tcpsock.go			*
├── tlssock.go			+
├── udpsock.go			*
└── unixsock.go			*

src/crypto/tls/
├── common.go			*
├── ticket.go			*
└── tls.go			*
```

The modifications to "net" are to basically wrap TCPConn, UDPConn, and TLSConn
around netdev socket calls.  In Go, these net.Conns call out to OS syscalls for
the socket operations.  In TinyGo, the OS syscalls aren't available, so netdev
socket calls are substituted.

The modifications to "net/http" are on the client and the server side.  On the
client side, the TinyGo code changes remove the back-end round-tripper code and
replaces it with direct calls to TCPConns/TLSConns.  All of Go's http
request/response handling code is intact and operational in TinyGo.  Same holds
true for the server side.  The server side supports the normal server features
like ServeMux and Hijacker (for websockets).

## Maintaining "net"

As Go progresses, changes to the "net" package need to be periodically
back-ported to TinyGo's "net" package.  This is to pick up any upstream bug
fixes or security fixes.

Changes "net" package files are marked with // TINYGO comments.

The files that are marked modified * may contain only a subset of the original
file.  Basically only the parts necessary to compile and run the example/net
examples are copied (and maybe modified).

### Upgrade Steps

Let's define some versions:

MIN = TinyGo minimum Go version supported (e.g. 1.15)
CUR = TinyGo "net" current version (e.g. 1.20.5)
UPSTREAM = Latest upstream Go version to upgrade to (e.g. 1.21)
NEW = TinyGo "net" new version, after upgrade

In example, we'll upgrade from CUR (1.20.5) to UPSTREAM (1.21).

These are the steps to promote TinyGos "net" to latest Go upstream version.
These steps should be done when:

- MIN moved forward
- TinyGo major release
- TinyGo minor release to pick up security fixes in UPSTREAM

Step 1:

Backport differences from Go UPSTREAM to Go CUR.  Since TinyGo CUR isn't the
full Go "net" implementation, only backport differences, don't add new stuff
from UPSTREAM (unless it's needed in the NEW release).

	NEW = CUR + diff(CUR, UPSTREAM)

If NEW contains updates not compatible with MIN, then NEW will need to revert
just those updates back to the CUR version, and annotate with a TINYGO comment.
If MIN moves forord, NEW can pull in the UPSTREAM changes.

Step 2:

As a double check, compare NEW against UPSTREAM.  The only differences at this
point should be excluded (not ported) code from UPSTREAM that wasn't in CUR in
the first place, and differences due to changes held back for MIN support.

Step 3:

Test NEW against example/net examples.  If everything checks out, then CUR
becomes NEW, and we can push to TinyGo.

	CUR = NEW
