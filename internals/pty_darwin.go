package internals

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"syscall"
	"unsafe"
)

const (
	TCGETS = syscall.TIOCGETA
	TCSETS = syscall.TIOCSETA
)

type pty_darwin struct {
	master *os.File  /* pty file descriptor*/
	slave  *os.File  /* tty file descriptor*/
	cmd    *exec.Cmd /* cmd */

	shutdown   chan struct{}
	term       *syscall.Termios
	wg         *sync.WaitGroup
	term_flags uintptr
	stdin      io.Reader
	stdout     io.Writer
}

type window_size struct {
	rows uint16
	cols uint16
	/* Not used */
	x uint16
	y uint16
}

func New(c *exec.Cmd) (*pty_darwin, error) {
	pty := &pty_darwin{
		cmd:      c,
		shutdown: make(chan struct{}),
		wg:       &sync.WaitGroup{},
		term:     &syscall.Termios{},
		stdin:    c.Stdin,
		stdout:   c.Stdout,
	}
	if err := pty.create_pty(); err != nil {
		return nil, err
	}
	if err := pty.setupStdin(); err != nil {
		pty.restore_stdin()
		return nil, err
	}
	if err := pty.cmd.Start(); err != nil {
		pty.restore_stdin()
		return nil, err
	}
	pty.wg.Add(2)
	go pty.read_thread()
	go pty.write_thread()
	return pty, nil
}

func (pty *pty_darwin) SetSize(rows, cols uint16) error {
	ws := &window_size{
		rows: rows,
		cols: cols,
	}
	return ioctl(pty.master, syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(ws)))
}

func (pty *pty_darwin) GetSize() (uint16, uint16, error) {
	return getSize(pty.master)
}

func (pty *pty_darwin) Wait() <-chan error {
	errs := make(chan error, 1)
	go func() {
		defer close(errs)
		if err := pty.cmd.Wait(); err != nil {
			pty.Close()
			errs <- err
			return
		}
		errs <- pty.Close()
	}()
	return errs
}

func (pty *pty_darwin) Close() error {
	pty.cmd.Process.Kill()
	if !isClosed(pty.shutdown) {
		close(pty.shutdown)
	}
	/* Best effort */
	pty.restore_stdin()
	if err := pty.master.Close(); err != nil {
		pty.slave.Close()
		return err
	}
	if err := pty.slave.Close(); err != nil {
		return err
	}
	pty.wg.Wait()
	return nil
}

func (pty *pty_darwin) create_pty() error {
	posix_openpt := func() (*os.File, error) {
		fd, err := syscall.Open("/dev/ptmx", syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOCTTY, 0)
		if err != nil {
			return nil, err
		}
		master := os.NewFile(uintptr(fd), "/dev/ptmx")
		return master, nil
	}
	grantpt := func(f *os.File) error {
		var u int32
		return ioctl(f, syscall.TIOCPTYGRANT, uintptr(unsafe.Pointer(&u)))
	}
	unlockpt := func(f *os.File) error {
		var u int32
		return ioctl(pty.master, syscall.TIOCPTYUNLK, uintptr(unsafe.Pointer(&u)))
	}
	ptsname := func(f *os.File) (string, error) {
		length := (syscall.TIOCPTYGNAME >> 16) & ((1 << 13) - 1)
		n := make([]byte, length)
		err := ioctl(f, syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&n[0])))
		if err != nil {
			return "", err
		}
		for i := range n {
			if n[i] == 0 {
				return string(n[:i]), nil
			}
		}
		return "", errors.New("ptsname not Null terminated?")
	}
	var err error

	/* posix_openpt() */
	pty.master, err = posix_openpt()
	if err != nil {
		return err
	}
	if err := grantpt(pty.master); err != nil {
		return err
	}

	if unlockpt(pty.master); err != nil {
		return err
	}

	sname, err := ptsname(pty.master)
	if err != nil {
		return err
	}

	pty.slave, err = os.OpenFile(sname, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	pty.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid:  true,
		Setctty: false,
	}
	pty.cmd.Stdout = pty.slave
	pty.cmd.Stderr = pty.slave
	pty.cmd.Stdin = pty.slave

	return nil
}

func (pty *pty_darwin) setupStdin() error {
	var err error

	if pty.stdin == nil {
		return nil
	}
	if f, ok := pty.stdin.(*os.File); ok {
		/* Sets stdin in non-blocking mode */
		pty.term_flags, _, err = fcntl(f, syscall.F_GETFL, 0)
		if err != nil {
			return err
		}
		_, _, err = fcntl(f, syscall.F_SETFL, pty.term_flags|syscall.O_NONBLOCK)
		if err != nil {
			return err
		}

		if f == os.Stdin {
			if pty.term, err = Cfmakeraw(f); err != nil {
				return err
			}
		}
		rows, cols, err := getSize(os.Stdin)
		if err != nil {
			return err
		}

		if rows == 0 || cols == 0 {
			cols = 80
			rows = 24
		}

		return pty.SetSize(rows, cols)
	}
	return nil
}

func (pty *pty_darwin) read_thread() {
	defer pty.wg.Done()
	if pty.stdout == nil {
		return
	}
	buff := make([]byte, 1<<10)
	for {
		n, err := pty.Read(buff)
		if err != nil {
			break
		}
		pty.stdout.Write(buff[:n])
	}
}

func (pty *pty_darwin) write_thread() {
	defer pty.wg.Done()
	if pty.stdin == nil {
		return
	}
	buff := make([]byte, 1<<7)
	for {
		select {
		case <-pty.shutdown:
			return
		default:
			n, _ := pty.stdin.Read(buff)
			if n == 0 {
				time.Sleep(25 * time.Millisecond)
				continue
			}
			pty.Write(buff[:n])
		}
	}
}

func (pty *pty_darwin) Write(buff []byte) (int, error) {
	return pty.master.Write(buff)
}

func (pty *pty_darwin) restore_stdin() {
	if f, ok := pty.stdin.(*os.File); ok {
		if pty.term != nil {
			Restor(f, pty.term)
		}
		if pty.term_flags != 0 {
			fcntl(f, syscall.F_SETFL, pty.term_flags)
		}
	}
}

func (pty *pty_darwin) Read(buff []byte) (int, error) {
	return pty.master.Read(buff)
}

func getSize(file *os.File) (uint16, uint16, error) {
	ws := &window_size{}
	err := ioctl(file, syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(ws)))
	return ws.rows, ws.cols, err
}

/*
   The ioctl() system call manipulates the underlying device parameters
   of special files.  In particular, many operating characteristics of
   character special files (e.g., terminals) may be controlled with
   ioctl() requests.
   - man ioctl
*/
func ioctl(f *os.File, cmd, ptr uintptr) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), cmd, ptr)
	if errno != 0 {
		return syscall.Errno(errno)
	}
	return nil
}

/* see man fcntl - hard to summarize */
func fcntl(f *os.File, cmd, ptr uintptr) (uintptr, uintptr, error) {
	a, b, errno := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), cmd, ptr)
	if errno != 0 {
		return a, b, syscall.Errno(errno)
	}
	return a, b, nil
}

func isClosed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
	}

	return false
}
