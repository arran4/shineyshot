package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type commandList []string

func (c *commandList) String() string {
	return strings.Join(*c, ";")
}

func (c *commandList) Set(value string) error {
	*c = append(*c, value)
	return nil
}

func writef(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}

func writeln(w io.Writer, msg string) error {
	_, err := fmt.Fprintln(w, msg)
	return err
}

func closeWithLog(name string, c io.Closer) {
	if err := c.Close(); err != nil {
		log.Printf("%s: close: %v", name, err)
	}
}

func removeWithLog(path string) {
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("remove %s: %v", path, err)
	}
}

type backgroundCmd struct {
	*root

	fs *flag.FlagSet

	op            string
	name          string
	dir           string
	helpRequested bool

	runArgs []string
}

func parseBackgroundCmd(args []string, r *root) (*backgroundCmd, error) {
	cmd := &backgroundCmd{root: r}
	if len(args) == 0 {
		cmd.fs = flag.NewFlagSet("background", flag.ExitOnError)
		cmd.fs.Usage = usageFunc(cmd)
		return nil, &UsageError{of: cmd}
	}
	cmd.op = strings.ToLower(args[0])
	cmd.fs = flag.NewFlagSet("background "+cmd.op, flag.ExitOnError)
	cmd.fs.Usage = usageFunc(cmd)

	switch cmd.op {
	case "start", "stop", "attach", "run", "serve":
		cmd.fs.StringVar(&cmd.name, "name", "", "socket session name")
	}
	switch cmd.op {
	case "start", "stop", "attach", "list", "clean", "run", "serve":
		cmd.fs.StringVar(&cmd.dir, "dir", "", "directory that stores shineyshot sockets")
	}
	cmd.fs.BoolVar(&cmd.helpRequested, "help", false, "show this help message and exit")

	if err := cmd.fs.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, &UsageError{of: cmd}
		}
		return nil, err
	}

	if cmd.helpRequested {
		return nil, &UsageError{of: cmd}
	}

	rest := cmd.fs.Args()

	switch cmd.op {
	case "start":
		if cmd.name == "" && len(rest) > 0 {
			cmd.name = rest[0]
			rest = rest[1:]
		}
		if cmd.dir == "" && len(rest) > 0 {
			cmd.dir = rest[0]
			rest = rest[1:]
		}
	case "stop", "attach":
		if cmd.name == "" && len(rest) > 0 {
			cmd.name = rest[0]
			rest = rest[1:]
		}
		if cmd.dir == "" && len(rest) > 0 {
			cmd.dir = rest[0]
			rest = rest[1:]
		}
	case "run":
		cmd.runArgs = append(cmd.runArgs, rest...)
		rest = nil
	case "list", "clean":
		if cmd.dir == "" && len(rest) > 0 {
			cmd.dir = rest[0]
			rest = rest[1:]
		}
	case "serve":
		if cmd.name == "" && len(rest) > 0 {
			cmd.name = rest[0]
			rest = rest[1:]
		}
		if cmd.dir == "" && len(rest) > 0 {
			cmd.dir = rest[0]
			rest = rest[1:]
		}
	default:
		return nil, &UsageError{of: cmd}
	}

	if len(rest) > 0 {
		return nil, &UsageError{of: cmd}
	}

	switch cmd.op {
	case "run":
		if len(cmd.runArgs) == 0 {
			return nil, errors.New("background run requires a command")
		}
	case "serve":
		if cmd.name == "" {
			return nil, errors.New("serve requires a session name")
		}
	}

	return cmd, nil
}

func (b *backgroundCmd) Program() string {
	return b.root.Program()
}

func (b *backgroundCmd) FlagSet() *flag.FlagSet {
	return b.fs
}

func (b *backgroundCmd) Template() string {
	return "background.txt"
}

func (b *backgroundCmd) Run() error {
	switch b.op {
	case "list":
		dir, err := resolveSocketDir(b.dir)
		if err != nil {
			return err
		}
		return printSocketList(dir, os.Stdout)
	case "clean":
		dir, err := resolveSocketDir(b.dir)
		if err != nil {
			return err
		}
		return cleanSocketDir(dir, os.Stdout)
	case "start":
		dir, err := resolveSocketDir(b.dir)
		if err != nil {
			return err
		}
		name, err := startBackgroundServer(dir, b.name, b.root)
		if err != nil {
			return err
		}
		if err := writef(os.Stdout, "started background session %s at %s\n", name, socketPath(dir, name)); err != nil {
			return err
		}
		return nil
	case "stop":
		dir, err := resolveSocketDir(b.dir)
		if err != nil {
			return err
		}
		name, err := selectSocketForStop(dir, b.name)
		if err != nil {
			return err
		}
		if err := stopSocket(dir, name); err != nil {
			return err
		}
		if err := writef(os.Stdout, "stop requested for %s\n", name); err != nil {
			return err
		}
		return nil
	case "attach":
		dir, err := resolveSocketDir(b.dir)
		if err != nil {
			return err
		}
		name, err := selectRunningSocket(dir, b.name)
		if err != nil {
			return err
		}
		return attachSocket(dir, name, os.Stdin, os.Stdout, os.Stderr)
	case "run":
		dir, err := resolveSocketDir(b.dir)
		if err != nil {
			return err
		}
		return b.runCommand(dir)
	case "serve":
		dir := b.dir
		if dir == "" {
			var err error
			dir, err = resolveSocketDir("")
			if err != nil {
				return err
			}
		}
		return runSocketServer(dir, b.name, b.root)
	default:
		return &UsageError{of: b}
	}
}

func resolveSocketDir(explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if dir := os.Getenv("SHINEYSHOT_SOCKET_DIR"); dir != "" {
		return dir, nil
	}
	if runtime.GOOS != "windows" {
		if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
			return filepath.Join(dir, "shineyshot"), nil
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".shineyshot", "sockets"), nil
}

type socketStatus struct {
	name string
	file string
	err  error
}

func collectSocketStatuses(dir string) ([]socketStatus, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	statuses := make([]socketStatus, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeDir != 0 {
			continue
		}
		name := entry.Name()
		if entry.Type()&os.ModeSocket == 0 && !strings.HasSuffix(name, ".sock") {
			continue
		}
		trimmed := strings.TrimSuffix(name, ".sock")
		path := filepath.Join(dir, name)
		status := socketStatus{name: trimmed, file: name}
		if err := pingSocket(path); err != nil {
			status.err = normalizeSocketError(err)
		}
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool { return statuses[i].name < statuses[j].name })
	return statuses, nil
}

func printSocketList(dir string, out io.Writer) error {
	statuses, err := collectSocketStatuses(dir)
	if err != nil {
		return err
	}
	if len(statuses) == 0 {
		return writeln(out, "no sockets found")
	}
	if err := writeln(out, "available sockets:"); err != nil {
		return err
	}
	for _, st := range statuses {
		if st.err != nil {
			if err := writef(out, "  %s (dead: %v)\n", st.name, st.err); err != nil {
				return err
			}
		} else {
			if err := writef(out, "  %s\n", st.name); err != nil {
				return err
			}
		}
	}
	return nil
}

func cleanSocketDir(dir string, out io.Writer) error {
	statuses, err := collectSocketStatuses(dir)
	if err != nil {
		return err
	}
	if len(statuses) == 0 {
		return writeln(out, "no dead sockets found")
	}
	var removed []string
	for _, st := range statuses {
		if st.err == nil {
			continue
		}
		path := filepath.Join(dir, st.file)
		if err := os.Remove(path); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err := writef(out, "failed to remove %s: %v\n", st.name, err); err != nil {
				return err
			}
			continue
		}
		removed = append(removed, st.name)
	}
	if len(removed) == 0 {
		if err := writeln(out, "no dead sockets found"); err != nil {
			return err
		}
	} else {
		if err := writef(out, "removed %d dead socket(s): %s\n", len(removed), strings.Join(removed, ", ")); err != nil {
			return err
		}
	}
	return nil
}

func socketPath(dir, name string) string {
	filename := name
	if !strings.HasSuffix(filename, ".sock") {
		filename += ".sock"
	}
	return filepath.Join(dir, filename)
}

func ensureSocketDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

func startBackgroundServer(dir, desiredName string, r *root) (string, error) {
	if err := ensureSocketDir(dir); err != nil {
		return "", err
	}
	name := desiredName
	if name == "" {
		var err error
		name, err = nextSocketName(dir)
		if err != nil {
			return "", err
		}
	}
	statuses, err := collectSocketStatuses(dir)
	if err != nil {
		return "", err
	}
	for _, st := range statuses {
		if st.name != name {
			continue
		}
		if st.err == nil {
			return "", fmt.Errorf("session %s already running", name)
		}
		path := filepath.Join(dir, st.file)
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		break
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	cmd := exec.Command(exe, "background", "serve", "--name", name, "--dir", dir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	if err := cmd.Process.Release(); err != nil {
		return "", err
	}
	socket := socketPath(dir, name)
	deadline := time.Now().Add(3 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := pingSocket(socket); err != nil {
			lastErr = normalizeSocketError(err)
			time.Sleep(50 * time.Millisecond)
			continue
		}
		return name, nil
	}
	if lastErr == nil {
		lastErr = errors.New("unknown startup failure")
	}
	return "", fmt.Errorf("session %s did not become ready: %v", name, lastErr)
}

func selectRunningSocket(dir, preferred string) (string, error) {
	statuses, err := collectSocketStatuses(dir)
	if err != nil {
		return "", err
	}
	alive := make([]string, 0, len(statuses))
	for _, st := range statuses {
		if st.err == nil {
			alive = append(alive, st.name)
		}
	}
	sort.Strings(alive)
	if preferred != "" {
		for _, name := range alive {
			if name == preferred {
				return preferred, nil
			}
		}
		return "", fmt.Errorf("session %s is not running", preferred)
	}
	switch len(alive) {
	case 0:
		return "", errors.New("no background sessions running")
	case 1:
		return alive[0], nil
	default:
		return "", fmt.Errorf("multiple background sessions running; specify a session name (%s)", strings.Join(alive, ", "))
	}
}

func selectSocketForStop(dir, preferred string) (string, error) {
	if preferred != "" {
		return preferred, nil
	}
	statuses, err := collectSocketStatuses(dir)
	if err != nil {
		return "", err
	}
	if len(statuses) == 0 {
		return "", errors.New("no background sessions found")
	}
	if len(statuses) == 1 {
		return statuses[0].name, nil
	}
	alive := make([]string, 0, len(statuses))
	for _, st := range statuses {
		if st.err == nil {
			alive = append(alive, st.name)
		}
	}
	if len(alive) == 1 {
		return alive[0], nil
	}
	return "", fmt.Errorf("multiple background sessions found; specify a session name (%s)", formatStatusNames(statuses))
}

func resolveRunTarget(dir, preferred string, args []string) (string, []string, error) {
	statuses, err := collectSocketStatuses(dir)
	if err != nil {
		return "", nil, err
	}
	alive := make(map[string]struct{}, len(statuses))
	for _, st := range statuses {
		if st.err == nil {
			alive[st.name] = struct{}{}
		}
	}
	name := preferred
	rest := append([]string(nil), args...)
	if len(rest) > 0 && name == "" {
		if _, ok := alive[rest[0]]; ok {
			name = rest[0]
			rest = rest[1:]
		}
	}
	if len(rest) == 0 {
		return "", nil, errors.New("background run requires a command")
	}
	if name == "" {
		switch len(alive) {
		case 0:
			return "", nil, errors.New("no background sessions running")
		case 1:
			for candidate := range alive {
				name = candidate
			}
		default:
			names := make([]string, 0, len(alive))
			for candidate := range alive {
				names = append(names, candidate)
			}
			sort.Strings(names)
			return "", nil, fmt.Errorf("multiple background sessions running; specify a session name (%s)", strings.Join(names, ", "))
		}
	} else {
		if _, ok := alive[name]; !ok {
			return "", nil, fmt.Errorf("session %s is not running", name)
		}
	}
	return name, rest, nil
}

func (b *backgroundCmd) runCommand(dir string) error {
	name, commandArgs, err := resolveRunTarget(dir, b.name, b.runArgs)
	if err != nil {
		return err
	}
	command := strings.Join(commandArgs, " ")
	return runSocketCommands(dir, name, []string{command}, os.Stdout, os.Stderr)
}

func formatStatusNames(statuses []socketStatus) string {
	names := make([]string, 0, len(statuses))
	for _, st := range statuses {
		names = append(names, st.name)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func nextSocketName(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "1", nil
		}
		return "", err
	}
	maxVal := 0
	for _, entry := range entries {
		if entry.Type()&os.ModeDir != 0 {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".sock")
		if val, err := strconv.Atoi(name); err == nil && val > maxVal {
			maxVal = val
		}
	}
	return strconv.Itoa(maxVal + 1), nil
}

func pingSocket(path string) error {
	conn, err := net.DialTimeout("unix", path, time.Second)
	if err != nil {
		return err
	}
	defer closeWithLog("ping socket", conn)
	if err := conn.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		return err
	}
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		return errors.New("socket closed")
	}
	if scanner.Text() != "READY" {
		return fmt.Errorf("unexpected greeting: %s", scanner.Text())
	}
	if _, err := fmt.Fprintln(conn, "PING"); err != nil {
		return err
	}
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		return errors.New("no pong received")
	}
	if scanner.Text() != "PONG" {
		return fmt.Errorf("unexpected response: %s", scanner.Text())
	}
	return nil
}

func normalizeSocketError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return errors.New("missing socket file")
	}
	if errors.Is(err, os.ErrPermission) {
		return errors.New("permission denied")
	}
	return err
}

type taggedWriter struct {
	w   io.Writer
	tag string
}

func (t *taggedWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	buf := make([]byte, len(t.tag)+len(p))
	copy(buf, t.tag)
	copy(buf[len(t.tag):], p)
	if _, err := t.w.Write(buf); err != nil {
		return 0, err
	}
	return len(p), nil
}

type interactiveSocketServer struct {
	session  *interactiveCmd
	path     string
	stopCh   chan struct{}
	listener net.Listener
	execMu   sync.Mutex
}

func runSocketServer(dir, name string, r *root) error {
	if err := ensureSocketDir(dir); err != nil {
		return err
	}
	path := socketPath(dir, name)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	server := &interactiveSocketServer{
		session: newInteractiveCmd(r),
		path:    path,
		stopCh:  make(chan struct{}),
	}
	return server.run()
}

func (s *interactiveSocketServer) run() error {
	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return err
	}
	s.listener = ln
	defer closeWithLog("socket listener", ln)
	defer removeWithLog(s.path)
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return nil
			default:
			}
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				continue
			}
			return err
		}
		go s.handleConn(conn)
	}
}

func (s *interactiveSocketServer) handleConn(conn net.Conn) {
	defer closeWithLog("socket connection", conn)
	if err := writeln(conn, "READY"); err != nil {
		log.Printf("socket write READY: %v", err)
		return
	}
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "PING":
			if err := writeln(conn, "PONG"); err != nil {
				log.Printf("socket write PONG: %v", err)
				return
			}
		case line == "SHUTDOWN":
			if err := writeln(conn, "DONE OK CLOSE"); err != nil {
				log.Printf("socket write DONE OK CLOSE: %v", err)
			}
			s.shutdown()
			return
		case strings.HasPrefix(line, "EXEC "):
			command := strings.TrimPrefix(line, "EXEC ")
			s.execMu.Lock()
			out := &taggedWriter{w: conn, tag: "OUT "}
			errW := &taggedWriter{w: conn, tag: "ERR "}
			restore := s.session.withIO(nil, out, errW)
			done, execErr := s.session.executeLine(command)
			restore()
			s.execMu.Unlock()
			if execErr != nil {
				msg := strings.ReplaceAll(execErr.Error(), "\n", "\\n")
				if err := writef(conn, "DONE ERR %s\n", msg); err != nil {
					log.Printf("socket write DONE ERR: %v", err)
					return
				}
				continue
			}
			if done {
				if err := writeln(conn, "DONE OK CLOSE"); err != nil {
					log.Printf("socket write DONE OK CLOSE: %v", err)
				}
				return
			}
			if err := writeln(conn, "DONE OK"); err != nil {
				log.Printf("socket write DONE OK: %v", err)
				return
			}
		default:
			if err := writeln(conn, "ERR unknown request"); err != nil {
				log.Printf("socket write error: %v", err)
				return
			}
		}
	}
}

func (s *interactiveSocketServer) shutdown() {
	select {
	case <-s.stopCh:
		return
	default:
	}
	close(s.stopCh)
	if s.listener != nil {
		closeWithLog("socket listener", s.listener)
	}
	removeWithLog(s.path)
}

func runSocketCommands(dir, name string, commands []string, stdout, stderr io.Writer) error {
	conn, err := net.Dial("unix", socketPath(dir, name))
	if err != nil {
		return err
	}
	defer closeWithLog("socket client", conn)
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		return errors.New("socket closed")
	}
	if scanner.Text() != "READY" {
		return fmt.Errorf("unexpected greeting: %s", scanner.Text())
	}
	for _, cmd := range commands {
		if err := executeOverSocket(conn, scanner, cmd, stdout, stderr); err != nil {
			if errors.Is(err, errSocketClosed) {
				return nil
			}
			return err
		}
	}
	return nil
}

func executeOverSocket(conn net.Conn, scanner *bufio.Scanner, cmd string, stdout, stderr io.Writer) error {
	if _, err := fmt.Fprintf(conn, "EXEC %s\n", cmd); err != nil {
		return err
	}
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "OUT "):
			if err := writeln(stdout, strings.TrimPrefix(line, "OUT ")); err != nil {
				return err
			}
		case strings.HasPrefix(line, "ERR "):
			if err := writeln(stderr, strings.TrimPrefix(line, "ERR ")); err != nil {
				return err
			}
		case strings.HasPrefix(line, "DONE OK"):
			if strings.HasSuffix(line, "CLOSE") {
				return errSocketClosed
			}
			return nil
		case strings.HasPrefix(line, "DONE ERR "):
			msg := strings.TrimPrefix(line, "DONE ERR ")
			return errors.New(strings.ReplaceAll(msg, "\\n", "\n"))
		default:
			if err := writeln(stdout, line); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return io.EOF
}

var errSocketClosed = errors.New("socket closed by server")

func attachSocket(dir, name string, stdin io.Reader, stdout, stderr io.Writer) error {
	conn, err := net.Dial("unix", socketPath(dir, name))
	if err != nil {
		return err
	}
	defer closeWithLog("socket client", conn)
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return err
		}
		return errors.New("socket closed")
	}
	if scanner.Text() != "READY" {
		return fmt.Errorf("unexpected greeting: %s", scanner.Text())
	}
	input := bufio.NewScanner(stdin)
	for {
		if _, err := fmt.Fprint(stdout, "> "); err != nil {
			return err
		}
		if !input.Scan() {
			return input.Err()
		}
		line := input.Text()
		if _, err := fmt.Fprintf(conn, "EXEC %s\n", line); err != nil {
			return err
		}
		if err := consumeSocketResponse(scanner, stdout, stderr); err != nil {
			if errors.Is(err, errSocketClosed) {
				return nil
			}
			return err
		}
	}
}

func consumeSocketResponse(scanner *bufio.Scanner, stdout, stderr io.Writer) error {
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "OUT "):
			if err := writeln(stdout, strings.TrimPrefix(line, "OUT ")); err != nil {
				return err
			}
		case strings.HasPrefix(line, "ERR "):
			if err := writeln(stderr, strings.TrimPrefix(line, "ERR ")); err != nil {
				return err
			}
		case strings.HasPrefix(line, "DONE OK"):
			if strings.HasSuffix(line, "CLOSE") {
				return errSocketClosed
			}
			return nil
		case strings.HasPrefix(line, "DONE ERR "):
			msg := strings.TrimPrefix(line, "DONE ERR ")
			return errors.New(strings.ReplaceAll(msg, "\\n", "\n"))
		default:
			if err := writeln(stdout, line); err != nil {
				return err
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return errSocketClosed
}

func stopSocket(dir, name string) error {
	path := socketPath(dir, name)
	conn, err := net.Dial("unix", path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		rmErr := os.Remove(path)
		if rmErr == nil || errors.Is(rmErr, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer closeWithLog("socket client", conn)
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return scanner.Err()
	}
	if scanner.Text() != "READY" {
		return fmt.Errorf("unexpected greeting: %s", scanner.Text())
	}
	if _, err := fmt.Fprintln(conn, "SHUTDOWN"); err != nil {
		return err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "DONE ") {
			removeWithLog(path)
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	removeWithLog(path)
	return nil
}
