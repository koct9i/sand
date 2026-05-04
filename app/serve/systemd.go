package serve

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/koct9i/sand/systemd"
)

func ServeSystemd(ctx context.Context) error {
	sockets, err := systemd.GetListenSockets()
	if err != nil {
		return err
	}
	if len(sockets) != 1 {
		return fmt.Errorf("need one listen socket")
	}
	srv := NewServer()
	return srv.Serve(sockets[0])
}

func StartSystemd(ctx context.Context) error {
	bin, _ := os.Executable()
	unit := systemd.Unit{
		Name: "sand",
		Description: "Sand",
		ListenSocket: "/tmp/sand.sock",
		Command: bin,
		Args: []string{"app", "serve-systemd"},
	}
	startCmd, startArgs := unit.StartCommand()
	cmd := exec.CommandContext(ctx, startCmd, startArgs...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func StopSystemd(ctx context.Context) error {
	unit := systemd.Unit{
		Name: "sand",
	}
	startCmd, startArgs := unit.StopCommand()
	cmd := exec.CommandContext(ctx, startCmd, startArgs...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
