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
package wmi_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"www.velocidex.com/golang/velociraptor/json"
	wmi_parse "www.velocidex.com/golang/velociraptor/vql/windows/wmi/parse"
	"www.velocidex.com/golang/velociraptor/vtesting/goldie"
)

func TestParseMOF(t *testing.T) {
	fd, err := os.Open("fixtures/sample.txt")
	assert.NoError(t, err, "open file")

	data, err := ioutil.ReadAll(fd)
	assert.NoError(t, err, "ReadAll")

	mof, err := wmi_parse.Parse(string(data))
	assert.NoError(t, err, "Parse")

	encoded, err := json.MarshalIndent(mof.ToDict())
	assert.NoError(t, err, "json.MarshalIndent")

	goldie.Assert(t, "sample", encoded)
}
