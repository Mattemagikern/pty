package pty

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/Mattemagikern/pty/internals"
)

type Pty interface {
	SetSize(cols, rows uint16) error
	GetSize() (uint16, uint16, error)
	Wait() <-chan error
	Close() error

	Read([]byte) (int, error)
	Write([]byte) (int, error)
}

func New(cmd *exec.Cmd) (Pty, error) {
	return internals.New(cmd)
}

func Cfmakeraw(f *os.File) (*syscall.Termios, error) {
	return internals.Cfmakeraw(f)
}

func Restore(f *os.File, term *syscall.Termios) error {
	return internals.Restor(f, term)
}
