package main

import (
	"github.com/Mattemagikern/pty"

	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestNew(t *testing.T) {
	c := exec.Command(os.Getenv("SHELL"))
	pty, err := pty.New(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := pty.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestTerminalSize(t *testing.T) {
	c := exec.Command(os.Getenv("SHELL"))
	pty, err := pty.New(c)
	if err != nil {
		t.Fatal(err)
	}
	defer pty.Close()
	if err := pty.SetSize(10, 20); err != nil {
		t.Error(err)
	}
	rows, cols, err := pty.GetSize()
	if err != nil {
		t.Fatal(err)
	}
	if rows != 10 || cols != 20 {
		t.Error(fmt.Sprintf("rows=%d,cols=%d", rows, cols))
	}
}

func TestCommand(t *testing.T) {
	c := exec.Command("./echo.sh", "TestCommand")
	c.Stdout = ioutil.Discard
	pty, err := pty.New(c)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-pty.Wait():
		if err != nil {
			t.Fatal(err)
		}
	case <-time.NewTimer(2 * time.Second).C:
		t.Fatal("Timeout")
		pty.Close()
	}
}

func TestBufferedCommand(t *testing.T) {
	in := &bytes.Buffer{}
	b := &bytes.Buffer{}
	in.Write([]byte("SomeThing\n"))
	c := exec.Command("cat")
	c.Stdout = b
	c.Stdin = in
	pty, err := pty.New(c)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(100 * time.Millisecond)
	if err := pty.Close(); err != nil {
		t.Fatal(err)
	}
	if b.Len() == 0 {
		t.Fatal("No data received")
	}
}

func TestReadWriteCommand(t *testing.T) {
	payload := []byte("SomeOtherThing\n")
	buff := make([]byte, len(payload)*2)
	c := exec.Command("cat")
	pty, err := pty.New(c)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pty.Write(payload); err != nil {
		t.Error(err)
		pty.Close()
	}
	n, err := pty.Read(buff)
	if n == 0 {
		t.Error("No data received")
	}
	if err := pty.Close(); err != nil {
		t.Fatal(err)
	}
}
