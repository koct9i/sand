package ssh

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/go-logr/logr"

	gossh "golang.org/x/crypto/ssh"
)

type Command struct {
	*gossh.Session

	Logger logr.Logger

	Path string
	Args []string
	Env  []string
	Dir  string

	Redirects []string // pairs: redirect, filename
}

var (
	shellVarName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	shellRedirect = regexp.MustCompile(`^[0-9]*(>|>>|<|<<|<<-|<<<|<>|>\||>&|<&|&>|&>>)(|&[0-9]+|&-)$`)
)

type ShellCommand struct {
	strings.Builder
}

func (sc *ShellCommand) WriteQuote(b, s, e string) {
	sc.WriteString(b)
	sc.WriteString(strings.ReplaceAll(s, "'", `'"'"'`))
	sc.WriteString(e)
}

func (c *Command) Start() error {
	var cmd ShellCommand
	if c.Dir != "" {
		cmd.WriteQuote(`cd '`, c.Dir, `' && `)
	}
	for _, env := range c.Env {
		key, val, found := strings.Cut(env, `=`)
		if !found || !shellVarName.MatchString(key) {
			return fmt.Errorf("invalid env: %q", key)
		}
		cmd.WriteString(key)
		cmd.WriteQuote(`='`, val, `' `)
	}
	cmd.WriteString(`exec`)
	for i, arg := range c.Args {
		if i == 0 && arg != c.Path {
			cmd.WriteQuote(` -a '`, arg, `'`)
			arg = c.Path
		}
		cmd.WriteQuote(` '`, arg, `'`)
	}
	for i := 0; i+1 < len(c.Redirects); i += 2 {
		redirect := c.Redirects[i]
		if !shellRedirect.MatchString(redirect) {
			return fmt.Errorf("invalid redirect: %q", redirect)
		}
		cmd.WriteString(` `)
		cmd.WriteString(redirect)
		if filename := c.Redirects[i+1]; filename != "" {
			cmd.WriteQuote(` '`, filename, `'`)
		}
	}
	c.Logger.Info("Remote command starting", "command", cmd.String())
	return c.Session.Start(cmd.String())
}

func (c *Command) Wait() error {
	err := c.Session.Wait()
	if err == nil {
		c.Logger.Info("Remote command finished")
	} else {
		c.Logger.Error(err, "Remote command failed")
	}
	return err
}

func (c *Command) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}
