//go:build linux

package proxy

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"

	"golang.org/x/sys/unix"
)

const soOriginalDst = 80

func GetOriginalDst(conn *net.TCPConn) (string, error) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return "", fmt.Errorf("acquire raw connection: %w", err)
	}

	var originalDst string
	var controlErr error

	err = rawConn.Control(func(fd uintptr) {
		addr, getsockErr := unix.GetsockoptIPv6Mreq(int(fd), unix.IPPROTO_IP, soOriginalDst)
		if getsockErr != nil {
			controlErr = getsockErr
			return
		}

		ip := net.IPv4(addr.Multiaddr[4], addr.Multiaddr[5], addr.Multiaddr[6], addr.Multiaddr[7])
		port := binary.BigEndian.Uint16(addr.Multiaddr[2:4])
		originalDst = net.JoinHostPort(ip.String(), strconv.Itoa(int(port)))
	})
	if err != nil {
		return "", fmt.Errorf("run control callback: %w", err)
	}

	if controlErr != nil {
		return "", fmt.Errorf("read SO_ORIGINAL_DST: %w", controlErr)
	}

	if originalDst == "" {
		return "", fmt.Errorf("original destination is empty")
	}

	return originalDst, nil
}
