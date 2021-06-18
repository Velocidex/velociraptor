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
/* An internal package with test utilities.
 */

package vtesting

import (
	"io/ioutil"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"www.velocidex.com/golang/velociraptor/utils"
)

func ReadFile(t *testing.T, filename string) []byte {
	result, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed reading file: %v", err)
	}
	return result
}

// Compares lists of strings regardless of order.
func CompareStrings(expected []string, watched []string) bool {
	if len(expected) != len(watched) {
		return false
	}

	for _, item := range watched {
		if !utils.InString(expected, item) {
			return false
		}
	}
	return true
}

func ContainsString(expected string, watched []string) bool {
	for _, line := range watched {
		if strings.Contains(line, expected) {
			return true
		}
	}
	return false
}

func WaitUntil(deadline time.Duration, t *testing.T, cb func() bool) {
	end_time := time.Now().Add(deadline)

	for end_time.After(time.Now()) {
		ok := cb()
		if ok {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("Timed out " + string(debug.Stack()))
}
