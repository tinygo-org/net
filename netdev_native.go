//go:build linux && !baremetal && !nintendoswitch && !wasm_unknown && !tinygo.wasm

// TINYGO: Native (host) netdev for the TinyGo "linux" target.
//
// On the native linux target TinyGo does NOT override the "syscall" package, so
// the standard library's syscall.Socket/Connect/Bind/... are available and the
// TinyGo compiler lowers syscall.Syscall/RawSyscall into real inline-asm system
// calls (see compiler/syscall.go). That means we can implement the netdever
// interface directly on top of raw Linux sockets, without needing a network
// driver or musl's (omitted) src/network module.
//
// This file registers that implementation as the default netdev, so that
// net.Dial/Listen/Lookup just work on a regular Linux host. See
// https://github.com/skycoin/skycoin/issues/2902.

package net

import (
	"io"
	"net/netip"
	"os"
	"strings"
	"syscall"
	"time"
)

// Register the host netdev as the default. A network driver (e.g. on a board
// that also reports GOOS=linux, which doesn't happen today) could still replace
// it by calling useNetdev() from its own init/setup.
func init() {
	useNetdev(&hostNetdev{})
}

// hostNetdev implements netdever using raw Linux sockets via the syscall
// package. The "sockfd" values it returns are plain OS file descriptors.
//
// Deadlines are implemented with the per-socket SO_RCVTIMEO/SO_SNDTIMEO
// options rather than a runtime poller. As a consequence, issuing concurrent
// reads (or concurrent writes) with different deadlines on the same connection
// is not supported; this matches the typical net.Conn usage of one reader and
// one writer goroutine.
type hostNetdev struct{}

// timeoutError is returned from Send/Recv when a deadline expires. It satisfies
// the net.Error interface so callers (e.g. net/http) can detect timeouts.
type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func (*hostNetdev) GetHostByName(name string) (netip.Addr, error) {
	return hostLookup(name)
}

func (*hostNetdev) Addr() (netip.Addr, error) {
	// Determine the address of the interface that would be used to reach the
	// public internet by "connecting" a UDP socket (no packets are sent) and
	// reading back the chosen local address.
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return netip.Addr{}, err
	}
	defer syscall.Close(fd)

	err = syscall.Connect(fd, &syscall.SockaddrInet4{
		Addr: [4]byte{8, 8, 8, 8},
		Port: 53,
	})
	if err != nil {
		// No route to the internet; fall back to loopback.
		return netip.AddrFrom4([4]byte{127, 0, 0, 1}), nil
	}

	sa, err := syscall.Getsockname(fd)
	if err != nil {
		return netip.Addr{}, err
	}
	if sa4, ok := sa.(*syscall.SockaddrInet4); ok {
		return netip.AddrFrom4(sa4.Addr), nil
	}
	return netip.AddrFrom4([4]byte{127, 0, 0, 1}), nil
}

func (*hostNetdev) Socket(domain, stype, protocol int) (int, error) {
	// _IPPROTO_TLS is a made-up protocol used by net.DialTLS on devices with an
	// offloaded TLS stack. The host has no such offload (TLS is done in Go via
	// crypto/tls over a plain TCP conn), so treat it as an ordinary TCP socket.
	if protocol == _IPPROTO_TLS {
		protocol = syscall.IPPROTO_TCP
	}

	// Non-blocking so the poller (netpoll_native.go) can park goroutines on
	// EAGAIN instead of pinning a thread in a blocking syscall.
	fd, err := syscall.Socket(domain, stype|syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC, protocol)
	if err != nil {
		return -1, err
	}

	// Allow quick rebind of listening sockets (e.g. restarting a server),
	// matching the standard library's behaviour.
	if stype == syscall.SOCK_STREAM {
		syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	}

	return fd, nil
}

func (*hostNetdev) Bind(sockfd int, ip netip.AddrPort) error {
	return syscall.Bind(sockfd, sockaddr(ip))
}

func (n *hostNetdev) Connect(sockfd int, host string, ip netip.AddrPort) error {
	addr := ip.Addr()
	if !addr.IsValid() || addr.IsUnspecified() {
		// net.DialTLS passes the host name with a zero IP; resolve it here.
		resolved, err := n.GetHostByName(host)
		if err != nil {
			return err
		}
		addr = resolved
	}
	sa := sockaddrFromParts(addr, ip.Port())
	for {
		err := syscall.Connect(sockfd, sa)
		switch err {
		case nil:
			return nil
		case syscall.EINTR:
			continue
		case syscall.EINPROGRESS, syscall.EALREADY, syscall.EAGAIN:
			// Non-blocking connect in progress: wait for the socket to become
			// writable, then read the pending error via SO_ERROR.
			if werr := poller.wait(sockfd, true, time.Time{}); werr != nil {
				return werr
			}
			soErr, gerr := syscall.GetsockoptInt(sockfd, syscall.SOL_SOCKET, syscall.SO_ERROR)
			if gerr != nil {
				return gerr
			}
			if soErr != 0 {
				return syscall.Errno(soErr)
			}
			return nil
		default:
			return err
		}
	}
}

func (*hostNetdev) Listen(sockfd int, backlog int) error {
	return syscall.Listen(sockfd, backlog)
}

func (*hostNetdev) Accept(sockfd int) (int, netip.AddrPort, error) {
	for {
		// Accept4 with SOCK_NONBLOCK|SOCK_CLOEXEC keeps accepted sockets
		// non-blocking too, so their reads/writes also go through the poller.
		nfd, sa, err := syscall.Accept4(sockfd, syscall.SOCK_NONBLOCK|syscall.SOCK_CLOEXEC)
		switch err {
		case nil:
		case syscall.EINTR:
			continue
		case syscall.EAGAIN: // == EWOULDBLOCK on Linux
			// No pending connection: park until the listener is readable or the
			// listening fd is closed (which unblocks accept for shutdown).
			if werr := poller.wait(sockfd, false, time.Time{}); werr != nil {
				return -1, netip.AddrPort{}, werr
			}
			continue
		default:
			return -1, netip.AddrPort{}, err
		}
		var raddr netip.AddrPort
		switch s := sa.(type) {
		case *syscall.SockaddrInet4:
			raddr = netip.AddrPortFrom(netip.AddrFrom4(s.Addr), uint16(s.Port))
		case *syscall.SockaddrInet6:
			raddr = netip.AddrPortFrom(netip.AddrFrom16(s.Addr), uint16(s.Port))
		}
		return nfd, raddr, nil
	}
}

func (*hostNetdev) Send(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	// net.Conn.Write must send the whole buffer (or report an error), so loop
	// over short writes. The send timeout is (re)programmed each iteration so a
	// deadline bounds the whole operation, not each individual write.
	total := 0
	for total < len(buf) {
		if expired(deadline) {
			return total, timeoutError{}
		}
		n, err := syscall.Write(sockfd, buf[total:])
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				// Send buffer full: park until writable, the deadline expires,
				// or the fd is closed.
				if werr := poller.wait(sockfd, true, deadline); werr != nil {
					return total, werr
				}
				continue
			}
			return total, err
		}
		if n <= 0 {
			break
		}
		total += n
	}
	return total, nil
}

func (*hostNetdev) Recv(sockfd int, buf []byte, flags int, deadline time.Time) (int, error) {
	if expired(deadline) {
		return 0, timeoutError{}
	}

	for {
		n, err := syscall.Read(sockfd, buf)
		if err != nil {
			if err == syscall.EINTR {
				continue
			}
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK {
				// Nothing to read yet: park until readable, the deadline
				// expires, or the fd is closed.
				if werr := poller.wait(sockfd, false, deadline); werr != nil {
					return 0, werr
				}
				continue
			}
			return n, err
		}
		// A read of 0 bytes on a stream socket means the peer closed the
		// connection. (A zero-length UDP datagram would also report EOF here;
		// the fd's socket type isn't tracked, but the net package only does
		// connected-UDP reads where this is not a practical concern.)
		if n == 0 && len(buf) > 0 {
			return 0, io.EOF
		}
		return n, nil
	}
}

func (*hostNetdev) Close(sockfd int) error {
	// Wake any goroutines parked on this fd (with errPollClosed) before closing
	// it, so a blocked Accept/Recv/Send returns promptly on shutdown instead of
	// hanging — which is what lets graceful shutdown and Ctrl+C complete.
	poller.close(sockfd)
	return syscall.Close(sockfd)
}

func (*hostNetdev) SetSockOpt(sockfd int, level int, opt int, value interface{}) error {
	// SO_LINGER takes a struct linger; the net package passes it the linger
	// seconds as an int.
	if level == syscall.SOL_SOCKET && opt == syscall.SO_LINGER {
		sec := toInt(value)
		l := &syscall.Linger{}
		if sec >= 0 {
			l.Onoff = 1
			l.Linger = int32(sec)
		}
		return syscall.SetsockoptLinger(sockfd, level, opt, l)
	}
	return syscall.SetsockoptInt(sockfd, level, opt, toInt(value))
}

// toInt coerces the values the net package passes to SetSockOpt (int, bool, or
// float64 durations) into an int.
func toInt(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case bool:
		if v {
			return 1
		}
		return 0
	case float64:
		return int(v)
	case int64:
		return int(v)
	default:
		return 0
	}
}

// sockaddr builds a syscall.Sockaddr from an AddrPort: a SockaddrInet6 for an
// IPv6 address, otherwise a SockaddrInet4 (an invalid/zero address maps to the
// 0.0.0.0 wildcard used when binding).
func sockaddr(ip netip.AddrPort) syscall.Sockaddr {
	return sockaddrFromParts(ip.Addr(), ip.Port())
}

func sockaddrFromParts(addr netip.Addr, port uint16) syscall.Sockaddr {
	// As4/As16 panic on a wrongly-sized address, so dispatch on the family.
	if addr = addr.Unmap(); addr.Is6() {
		// Link-local zones (%zone) are not resolved to a scope id.
		return &syscall.SockaddrInet6{Port: int(port), Addr: addr.As16()}
	}
	sa := &syscall.SockaddrInet4{Port: int(port)}
	if addr.Is4() {
		sa.Addr = addr.As4()
	}
	return sa
}

// expired reports whether a non-zero deadline is already in the past.
func expired(deadline time.Time) bool {
	return !deadline.IsZero() && !deadline.After(time.Now())
}

// setSockTimeout programs SO_RCVTIMEO/SO_SNDTIMEO so a blocking recv/send
// returns EAGAIN at the deadline. A zero deadline disables the timeout.
func setSockTimeout(sockfd int, opt int, deadline time.Time) error {
	var tv syscall.Timeval
	if !deadline.IsZero() {
		d := time.Until(deadline)
		if d < time.Microsecond {
			d = time.Microsecond
		}
		tv = syscall.NsecToTimeval(d.Nanoseconds())
	}
	return syscall.SetsockoptTimeval(sockfd, syscall.SOL_SOCKET, opt, &tv)
}

// --- Name resolution --------------------------------------------------------

// hostLookup resolves a host name (or IP literal) to a single IPv4 address,
// consulting (in order): IP literals, /etc/hosts, then DNS.
func hostLookup(name string) (netip.Addr, error) {
	if name == "" {
		return netip.AddrFrom4([4]byte{0, 0, 0, 0}), nil
	}

	// IP literal?
	if addr, err := netip.ParseAddr(name); err == nil {
		return addr.Unmap(), nil
	}

	// /etc/hosts
	if addr, ok := lookupStaticHost(name); ok {
		return addr, nil
	}

	// Well-known fallback in case /etc/hosts is missing.
	if strings.EqualFold(name, "localhost") {
		return netip.AddrFrom4([4]byte{127, 0, 0, 1}), nil
	}

	return dnsLookup(name)
}

// lookupStaticHost scans /etc/hosts for an address matching name, preferring an
// IPv4 match but falling back to IPv6.
func lookupStaticHost(name string) (netip.Addr, bool) {
	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return netip.Addr{}, false
	}
	var v6 netip.Addr
	var haveV6 bool
	for _, line := range strings.Split(string(data), "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		addr, err := netip.ParseAddr(fields[0])
		if err != nil {
			continue
		}
		addr = addr.Unmap()
		for _, h := range fields[1:] {
			if strings.EqualFold(h, name) {
				if addr.Is4() {
					return addr, true
				}
				if !haveV6 {
					v6, haveV6 = addr, true
				}
			}
		}
	}
	return v6, haveV6
}

// resolvConfServers returns the nameservers from /etc/resolv.conf as
// "ip:53" strings, defaulting to localhost if the file is missing/empty.
func resolvConfServers() []string {
	var servers []string
	if data, err := os.ReadFile("/etc/resolv.conf"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if i := strings.IndexByte(line, '#'); i >= 0 {
				line = line[:i]
			}
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "nameserver" {
				if addr, err := netip.ParseAddr(fields[1]); err == nil && addr.Unmap().Is4() {
					servers = append(servers, addr.Unmap().String())
				}
			}
		}
	}
	if len(servers) == 0 {
		servers = []string{"127.0.0.1"}
	}
	return servers
}

const (
	dnsTypeA    = 1
	dnsTypeAAAA = 28
)

// dnsLookup resolves name by querying the system nameservers over UDP,
// preferring an IPv4 (A) answer and falling back to IPv6 (AAAA).
func dnsLookup(name string) (netip.Addr, error) {
	if addr, err := dnsLookupType(name, dnsTypeA); err == nil {
		return addr, nil
	}
	if addr, err := dnsLookupType(name, dnsTypeAAAA); err == nil {
		return addr, nil
	}
	return netip.Addr{}, &DNSError{Err: "no address found", Name: name}
}

// dnsLookupType resolves name for a single DNS record type (A or AAAA).
func dnsLookupType(name string, qtype uint16) (netip.Addr, error) {
	id, query := buildDNSQuery(name, qtype)
	var lastErr error
	for _, server := range resolvConfServers() {
		addr, err := dnsQuery(server, query, id, qtype)
		if err == nil {
			return addr, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = &DNSError{Err: "no answer", Name: name}
	}
	return netip.Addr{}, lastErr
}

// dnsID derives a non-secret 16-bit query ID. It need not be cryptographically
// random for a stub resolver on a trusted link; it just disambiguates replies.
func dnsID() uint16 {
	return uint16(time.Now().UnixNano())
}

// buildDNSQuery builds a standard recursive query for name of the given record
// type and returns it together with the query ID, so the reply can be matched
// against it.
func buildDNSQuery(name string, qtype uint16) (uint16, []byte) {
	id := dnsID()
	msg := []byte{
		byte(id >> 8), byte(id), // ID
		0x01, 0x00, // flags: recursion desired
		0x00, 0x01, // QDCOUNT
		0x00, 0x00, // ANCOUNT
		0x00, 0x00, // NSCOUNT
		0x00, 0x00, // ARCOUNT
	}
	for _, label := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		if len(label) == 0 || len(label) > 63 {
			continue
		}
		msg = append(msg, byte(len(label)))
		msg = append(msg, label...)
	}
	msg = append(msg, 0x00) // root label
	msg = append(msg,
		byte(qtype>>8), byte(qtype), // QTYPE
		0x00, 0x01, // QCLASS = IN
	)
	return id, msg
}

// dnsQuery sends query to server (an IPv4 string) on port 53 and returns the
// first record of type qtype from the response that matches id.
func dnsQuery(server string, query []byte, id uint16, qtype uint16) (netip.Addr, error) {
	srv, err := netip.ParseAddr(server)
	if err != nil {
		return netip.Addr{}, err
	}

	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return netip.Addr{}, err
	}
	defer syscall.Close(fd)

	if err := syscall.Connect(fd, &syscall.SockaddrInet4{Addr: srv.As4(), Port: 53}); err != nil {
		return netip.Addr{}, err
	}

	// Bound how long we wait for a reply.
	tv := syscall.NsecToTimeval((5 * time.Second).Nanoseconds())
	syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)

	if _, err := syscall.Write(fd, query); err != nil {
		return netip.Addr{}, err
	}

	resp := make([]byte, 512)
	n, err := syscall.Read(fd, resp)
	if err != nil {
		return netip.Addr{}, err
	}
	return parseDNSResponse(resp[:n], id, qtype)
}

// parseDNSResponse extracts the first record of type qtype from a DNS response
// message, after validating that it is a non-error reply to query id.
func parseDNSResponse(msg []byte, id uint16, qtype uint16) (netip.Addr, error) {
	if len(msg) < 12 {
		return netip.Addr{}, &DNSError{Err: "short DNS response"}
	}
	// Match the reply to our query and check it is a response, not an error.
	if uint16(msg[0])<<8|uint16(msg[1]) != id {
		return netip.Addr{}, &DNSError{Err: "DNS response ID mismatch"}
	}
	if msg[2]&0x80 == 0 {
		return netip.Addr{}, &DNSError{Err: "DNS reply is not a response"}
	}
	switch rcode := msg[3] & 0x0f; rcode {
	case 0: // NOERROR
	case 3: // NXDOMAIN
		return netip.Addr{}, &DNSError{Err: "host not found", IsNotFound: true}
	default:
		return netip.Addr{}, &DNSError{Err: "DNS server error"}
	}
	qdcount := int(msg[4])<<8 | int(msg[5])
	ancount := int(msg[6])<<8 | int(msg[7])

	off := 12
	// Skip the question section.
	for i := 0; i < qdcount; i++ {
		off = skipName(msg, off)
		if off < 0 || off+4 > len(msg) {
			return netip.Addr{}, &DNSError{Err: "malformed DNS question"}
		}
		off += 4 // QTYPE + QCLASS
	}

	for i := 0; i < ancount; i++ {
		off = skipName(msg, off)
		if off < 0 || off+10 > len(msg) {
			return netip.Addr{}, &DNSError{Err: "malformed DNS answer"}
		}
		rrtype := int(msg[off])<<8 | int(msg[off+1])
		rdlength := int(msg[off+8])<<8 | int(msg[off+9])
		off += 10
		if off+rdlength > len(msg) {
			return netip.Addr{}, &DNSError{Err: "malformed DNS rdata"}
		}
		if rrtype == int(qtype) {
			if qtype == dnsTypeA && rdlength == 4 {
				var b [4]byte
				copy(b[:], msg[off:off+4])
				return netip.AddrFrom4(b), nil
			}
			if qtype == dnsTypeAAAA && rdlength == 16 {
				var b [16]byte
				copy(b[:], msg[off:off+16])
				return netip.AddrFrom16(b), nil
			}
		}
		off += rdlength
	}
	return netip.Addr{}, &DNSError{Err: "no matching record in DNS response", IsNotFound: true}
}

// skipName advances past a (possibly compressed) DNS name and returns the
// offset just after it, or -1 on malformed input.
func skipName(msg []byte, off int) int {
	for {
		if off >= len(msg) {
			return -1
		}
		b := int(msg[off])
		switch {
		case b == 0:
			return off + 1
		case b&0xc0 == 0xc0:
			// Compression pointer ends the name.
			return off + 2
		default:
			off += 1 + b
		}
	}
}
