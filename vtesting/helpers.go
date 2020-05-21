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
	"testing"
	"time"
)

func ReadFile(t *testing.T, filename string) []byte {
	result, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("Failed reading file: %v", err)
	}
	return result
}

type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

type RealClock struct{}

func (self RealClock) Now() time.Time {
	return time.Now()
}
func (self RealClock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

func WaitUntil(deadline time.Duration, t *testing.T, cb func() bool) {
	end_time := time.Now().Add(deadline)

	for end_time.After(time.Now()) {
		ok := cb()
		if ok {
			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("Timed out")
}
