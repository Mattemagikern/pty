package internals

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"syscall"
	"unsafe"
)

const (
	TCGETS = syscall.TCGETS
	TCSETS = syscall.TCSETS
)

type pty_linux struct {
	slave  *os.File  /* tty */
	master *os.File  /* pty file descriptor*/
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

func New(c *exec.Cmd) (*pty_linux, error) {
	pty := &pty_linux{
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
	if err := c.Start(); err != nil {
		pty.restore_stdin()
		return nil, err
	}
	pty.wg.Add(2)
	go pty.read_thread()
	go pty.write_thread()
	return pty, nil
}

func (pty *pty_linux) SetSize(rows, cols uint16) error {
	ws := &window_size{
		rows: rows,
		cols: cols,
	}
	return ioctl(pty.master, syscall.TIOCSWINSZ, uintptr(unsafe.Pointer(ws)))
}

func (pty *pty_linux) GetSize() (uint16, uint16, error) {
	return getSize(pty.master)
}

func (pty *pty_linux) Wait() <-chan error {
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

func (pty *pty_linux) Close() error {
	pty.cmd.Process.Kill()
	if !isClosed(pty.shutdown) {
		close(pty.shutdown)
	}
	/* Best effort */
	pty.restore_stdin()
	if err := pty.slave.Close(); err != nil {
		pty.master.Close()
		return err
	}
	if err := pty.master.Close(); err != nil {
		return err
	}
	pty.wg.Wait()
	return nil
}

func (pty *pty_linux) restore_stdin() {
	if f, ok := pty.stdin.(*os.File); ok {
		if pty.term != nil {
			Restor(f, pty.term)
		}
		if pty.term_flags != 0 {
			fcntl(f, syscall.F_SETFL, pty.term_flags)
		}
	}
}

func (pty *pty_linux) create_pty() error {
	var err error
	posix_openpt := func() (*os.File, error) {
		return os.OpenFile("/dev/ptmx", syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOCTTY, 0)
	}
	grantpt := func(f *os.File) error {
		/* grantpt is a no-op on Linux */
		return nil
	}
	unlockpt := func(f *os.File) error {
		/* unlock pty */
		var n uint32
		return ioctl(f, syscall.TIOCSPTLCK, uintptr(unsafe.Pointer(&n)))
	}
	ptsname := func(f *os.File) (string, error) {
		var n uint32
		err := ioctl(f, syscall.TIOCGPTN, uintptr(unsafe.Pointer(&n)))
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("/dev/pts/%d", uint(n)), nil
	}

	pty.master, err = posix_openpt()
	if err != nil {
		return err
	}

	if err := grantpt(pty.master); err != nil {
		return err
	}

	if err := unlockpt(pty.master); err != nil {
		return err
	}

	name, err := ptsname(pty.master)
	if err != nil {
		return err
	}

	pty.slave, err = os.OpenFile(name, os.O_RDWR, 0)
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

func (pty *pty_linux) setupStdin() error {
	var err error
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

func (pty *pty_linux) read_thread() {
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

func (pty *pty_linux) write_thread() {
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

func (pty *pty_linux) Write(buff []byte) (int, error) {
	return pty.master.Write(buff)
}

func (pty *pty_linux) Read(buff []byte) (int, error) {
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
