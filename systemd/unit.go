package systemd

type Unit struct {
	Name         string
	Description  string
	ListenSocket string
	Command      string
	Args         []string
}

// Link: https://www.freedesktop.org/software/systemd/man/latest/systemd-run.html
// Link: https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html
// Link: https://www.freedesktop.org/software/systemd/man/latest/systemd.socket.html
func (u *Unit) StartCommand() (string, []string) {
	return "systemd-run", append([]string{
		"--user",
		"--unit=" + u.Name,
		"--description=" + u.Description,
		"--property=Restart=on-failure",
		"--socket-property=ListenStream=" + u.ListenSocket,
		"--socket-property=SocketMode=0600",
		"--socket-property=NoDelay=true",
		u.Command,
	}, u.Args...)
}

func (u *Unit) StopCommand() (string, []string) {
	return "systemctl", []string{
		"--user",
		"stop",
		u.Name + ".socket",
		u.Name + ".service",
	}
}
