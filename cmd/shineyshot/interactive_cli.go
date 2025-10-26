package main

import "flag"

type interactiveCLI struct {
	*interactiveCmd

	fs *flag.FlagSet

	execs       commandList
	sessionName string
	socketDir   string
}

func parseInteractiveCmd(args []string, r *root) (*interactiveCLI, error) {
	base := newInteractiveCmd(r)
	fs := flag.NewFlagSet("interactive", flag.ExitOnError)
	cli := &interactiveCLI{interactiveCmd: base, fs: fs}
	fs.Usage = usageFunc(cli)
	fs.Var(&cli.execs, "e", "execute interactive command in immediate mode (may be specified multiple times)")
	fs.StringVar(&cli.sessionName, "name", "", "background session name")
	fs.StringVar(&cli.sessionName, "socket", "", "background session name (deprecated)")
	fs.StringVar(&cli.socketDir, "dir", "", "directory that stores shineyshot sockets")
	fs.StringVar(&cli.socketDir, "socket-dir", "", "directory that stores shineyshot sockets (deprecated)")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return cli, nil
}

func (c *interactiveCLI) FlagSet() *flag.FlagSet {
	return c.fs
}

func (c *interactiveCLI) Program() string {
	return c.r.Program()
}

func (c *interactiveCLI) Run() error {
	if len(c.execs) > 0 {
		if c.sessionName != "" {
			dir, err := resolveSocketDir(c.socketDir)
			if err != nil {
				return err
			}
			commands := make([]string, len(c.execs))
			copy(commands, c.execs)
			return runSocketCommands(dir, c.sessionName, commands, c.stdout, c.stderr)
		}
		for _, cmd := range c.execs {
			done, err := c.executeLine(cmd)
			if err != nil {
				return err
			}
			if done {
				break
			}
		}
		return nil
	}

	return c.interactiveCmd.Run()
}
