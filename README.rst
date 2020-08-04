######################
Pty - Pseudo terminal
######################

Pty implementation of UNIX 98 pty system.
pty wraps the os/exec library to handle the command execution wheres pty
will enable the os/exec module to run through a pseudo terminal.

Currently supports: Linux and Darwin(OSX).


How To:
------------

Pseudo terminal program:

.. code-block:: go

    package main

   import (
       "github.com/Mattemagikern/pty"

       "log"
       "os"
       "os/exec"
   )

   func pseudo_terminal() error {
       c := exec.Command("bash")
       /* Both Stdout and Stderr */
       c.Stdout = os.Stdout
       c.Stdin = os.Stdin

       pty, err := pty.New(c)
       if err != nil {
           return err
       }

       if err := <-pty.Wait(); err != nil {
           return err
       }
       return nil
   }

   func main() {
       if err := pseudo_terminal(); err != nil {
           log.Fatal(err)
       }
   }

Run interactive commands:

.. code-block:: go

   func run_cmd() error {
       c := exec.Command("cat")
       /* Both Stdout and Stderr */
       c.Stdout = os.Stdout
       c.Stdin = os.Stdin
       pty, err := pty.New(c)
       if err != nil {
           return err
       }

       if err := <-pty.Wait(); err != nil {
           return err
       }
       return nil
   }

   func main() {
       if err := run_cmd(); err != nil {
           log.Fatal(err)
       }
   }


Quirks
---------

An interesting thing I've found is that on Darwin you need to read the stdout
otherwise the execution stops. To see this behaviour; add the test below to the
pty_test.go file and execute on osx. Interesting enough this isn't a problem on
Linux.

.. code-block:: go

   func TestCommandDarwinQuirk(t *testing.T) {
      c := exec.Command("./echo.sh", "TestCommand")
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

