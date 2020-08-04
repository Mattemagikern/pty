// +build linux darwin

package internals

import (
	"os"
	"syscall"
	"unsafe"
)

func Cfmakeraw(f *os.File) (*syscall.Termios, error) {
	var raw_term, old_term syscall.Termios
	if err := ioctl(f, TCGETS, uintptr(unsafe.Pointer(&old_term))); err != nil {
		return nil, err
	}

	raw_term = old_term
	raw_term.Iflag = 0
	raw_term.Oflag = syscall.OPOST | syscall.ONLCR
	raw_term.Lflag = syscall.ECHOE | syscall.ECHOK | syscall.ECHOCTL | syscall.ECHOKE
	raw_term.Cflag &^= (syscall.CSIZE | syscall.PARENB)
	raw_term.Cflag |= syscall.CS8
	raw_term.Cc[syscall.VMIN] = 1
	raw_term.Cc[syscall.VTIME] = 0

	if err := ioctl(f, TCSETS, uintptr(unsafe.Pointer(&raw_term))); err != nil {
		return nil, err
	}
	return &old_term, nil
}

func Restor(f *os.File, term *syscall.Termios) error {
	return ioctl(f, TCSETS, uintptr(unsafe.Pointer(term)))
}
