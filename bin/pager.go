package main

import (
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/google/shlex"
)

type Pager struct {
	pager  *exec.Cmd
	Writer io.WriteCloser
	Reader io.ReadCloser
	wg     *sync.WaitGroup
}

func NewPager(command string) (*Pager, error) {
	self := &Pager{}

	// Create a pipe for a pager to use
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}

	self.Writer = w
	self.Reader = r

	argv, err := shlex.Split(command)
	if err != nil {
		return nil, err
	}

	argv_args := []string{}
	if len(argv) > 1 {
		argv_args = argv[1:len(argv)]
	}
	self.pager = exec.Command(argv[0], argv_args...)
	self.pager.Stdin = r
	self.pager.Stdout = os.Stdout
	self.pager.Stderr = os.Stderr
	self.wg = &sync.WaitGroup{}

	self.wg.Add(1)

	// Run the pager
	go func() {
		defer self.Close()
		defer self.wg.Done()

		err := self.pager.Run()
		if err != nil {
			ConsoleLog.Error("Error launching pager: %v\n", err)
		}
	}()

	return self, nil
}

func (self *Pager) Close() {
	self.Writer.Close()
	self.Reader.Close()

	self.wg.Wait()
}
