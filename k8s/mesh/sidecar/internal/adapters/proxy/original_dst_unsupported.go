//go:build !linux

package proxy

import (
	"fmt"
	"net"
)

func GetOriginalDst(_ *net.TCPConn) (string, error) {
	return "", fmt.Errorf("SO_ORIGINAL_DST is only supported on Linux")
}
