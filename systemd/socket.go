package systemd

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// Link: https://www.freedesktop.org/software/systemd/man/latest/sd_listen_fds.html
func GetListenSockets() ([]net.Listener, error) {
	const firstFD = 3
	pidStr := os.Getenv("LISTEN_PID")
	fdsStr := os.Getenv("LISTEN_FDS")
	if pidStr == "" || fdsStr == "" {
		return nil, fmt.Errorf("LISTEN_PID, LISTEN_FDS not found")
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return nil, fmt.Errorf("LISTEN_PID=%q: %w", pidStr, err)
	}
	if pid != os.Getpid() {
		return nil, fmt.Errorf("LISTEN_PID=%q does not match current pid=%v", pidStr, os.Getpid())
	}
	nfds, err := strconv.Atoi(fdsStr)
	if err != nil {
		return nil, fmt.Errorf("LISTEN_FDS=%q: %w", fdsStr, err)
	}
	var names []string
	if namesStr := os.Getenv("LISTEN_FDNAMES"); namesStr != "" {
		names = strings.Split(namesStr, ":")
	}
	for len(names) < nfds {
		names = append(names, fmt.Sprintf("fd%d", firstFD+len(names)))
	}

	listers := make([]net.Listener, nfds)
	for i := range nfds {
		fd := firstFD + i
		file := os.NewFile(uintptr(fd), names[i])
		if file == nil {
			for j := range i {
				listers[j].Close()
			}
			return nil, fmt.Errorf("failed to wrap fd %v", fd)
		}
		ln, err := net.FileListener(file)
		file.Close()
		if err != nil {
			for j := range i {
				listers[j].Close()
			}
			return nil, fmt.Errorf("failed to create listener from fd %v: %w", fd, err)
		}
		listers[i] = ln
	}

	os.Unsetenv("LISTEN_PID")
	os.Unsetenv("LISTEN_FDS")
	os.Unsetenv("LISTEN_FDNAMES")

	return listers, nil
}
