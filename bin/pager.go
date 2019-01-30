/*
   Velociraptor - Hunting Evil
   Copyright (C) 2019 Velocidex Innovations.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package main

import (
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/google/shlex"
	vfilter "www.velocidex.com/golang/vfilter"
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
	if err != nil || len(argv) == 0 {
		return nil, err
	}

	argv_args := []string{}
	if len(argv) > 1 {
		argv_args = argv[1:]
	}
	self.pager = exec.Command(argv[0], argv_args...)
	self.pager.Stdin = r
	self.pager.Stdout = os.Stdout
	self.pager.Stderr = os.Stderr
	self.wg = &sync.WaitGroup{}

	err = self.pager.Start()
	if err != nil {
		return nil, err
	}

	self.wg.Add(1)

	// Run the pager
	go func() {
		defer self.Close()
		defer self.wg.Done()

		err := self.pager.Wait()
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

func GetPager(scope *vfilter.Scope) (*Pager, error) {
	env_pager, pager_pres := scope.Resolve("PAGER")
	if pager_pres {
		pager_cmd, _ := env_pager.(string)
		pager, err := NewPager(pager_cmd)
		if err == nil {
			return pager, nil
		} else {
			return nil, err
		}
	}
	return nil, nil
}
