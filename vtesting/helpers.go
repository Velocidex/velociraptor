/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

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
	"www.velocidex.com/golang/vfilter/types"
)

type TestFailer interface {
	Fatalf(format string, args ...interface{})
}

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

func WaitUntil(deadline time.Duration, t TestFailer, cb func() bool) {
	// Use the real time for this because the time is likely to be
	// mocked in tests and we need to really wait here.
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

func RunPlugin(in <-chan types.Row) []types.Row {
	result := make([]types.Row, 0)
	for row := range in {
		result = append(result, row)
	}

	return result
}
