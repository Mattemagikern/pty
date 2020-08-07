package main

import (
	"github.com/Mattemagikern/pty"

	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

func TestCommand(t *testing.T) {
	goStart := runtime.NumGoroutine()
	c := exec.Command(os.Getenv("SHELL"))
	pty, err := pty.New(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := pty.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	goEnd := runtime.NumGoroutine()
	if goStart != goEnd {
		t.Error(fmt.Sprintf("Memory leak!start:%d end:%d", goStart, goEnd))
	}
}

func TestTerminalSize(t *testing.T) {
	goStart := runtime.NumGoroutine()
	c := exec.Command(os.Getenv("SHELL"))
	pty, err := pty.New(c)
	if err != nil {
		t.Fatal(err)
	}
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

	if err := pty.Close(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(25 * time.Millisecond)
	goEnd := runtime.NumGoroutine()
	if goStart != goEnd {
		t.Error(fmt.Sprintf("Memory leak!start:%d end:%d", goStart, goEnd))
	}
}

func TestScript(t *testing.T) {
	goStart := runtime.NumGoroutine()
	c := exec.Command("./echo.sh", "TestCommand")
	c.Stdout = &bytes.Buffer{} //discard
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
	time.Sleep(50 * time.Millisecond)
	goEnd := runtime.NumGoroutine()
	if goStart != goEnd {
		t.Error(fmt.Sprintf("Memory leak!start:%d end:%d", goStart, goEnd))
	}
}

func TestBufferedCommand(t *testing.T) {
	goStart := runtime.NumGoroutine()
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
	time.Sleep(25 * time.Millisecond)
	goEnd := runtime.NumGoroutine()
	if goStart != goEnd {
		t.Error(fmt.Sprintf("Memory leak!start:%d end:%d", goStart, goEnd))
	}
}

func TestReadWrite(t *testing.T) {
	goStart := runtime.NumGoroutine()
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
	time.Sleep(25 * time.Millisecond)
	goEnd := runtime.NumGoroutine()
	if goStart != goEnd {
		t.Error(fmt.Sprintf("Memory leak!start:%d end:%d", goStart, goEnd))
	}
}
