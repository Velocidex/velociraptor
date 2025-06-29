// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package csv

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

var writeTests = []struct {
	Input   [][]string
	Output  string
	UseCRLF bool
}{
	{Input: [][]string{{"abc"}}, Output: "abc\n"},
	{Input: [][]string{{"abc"}}, Output: "abc\r\n", UseCRLF: true},
	{Input: [][]string{{`"abc"`}}, Output: `"""abc"""` + "\n"},
	{Input: [][]string{{`a"b`}}, Output: `"a""b"` + "\n"},
	{Input: [][]string{{`"a"b"`}}, Output: `"""a""b"""` + "\n"},
	{Input: [][]string{{" abc"}}, Output: `" abc"` + "\n"},
	{Input: [][]string{{"abc,def"}}, Output: `"abc,def"` + "\n"},
	{Input: [][]string{{"abc", "def"}}, Output: "abc,def\n"},
	{Input: [][]string{{"abc"}, {"def"}}, Output: "abc\ndef\n"},
	{Input: [][]string{{"abc\ndef"}}, Output: "\"abc\ndef\"\n"},
	{Input: [][]string{{"abc\ndef"}}, Output: "\"abc\r\ndef\"\r\n", UseCRLF: true},
	{Input: [][]string{{"abc\rdef"}}, Output: "\"abcdef\"\r\n", UseCRLF: true},
	{Input: [][]string{{"abc\rdef"}}, Output: "\"abc\rdef\"\n", UseCRLF: false},
	{Input: [][]string{{""}}, Output: "\n"},
	{Input: [][]string{{"", ""}}, Output: ",\n"},
	{Input: [][]string{{"", "", ""}}, Output: ",,\n"},
	{Input: [][]string{{"", "", "a"}}, Output: ",,a\n"},
	{Input: [][]string{{"", "a", ""}}, Output: ",a,\n"},
	{Input: [][]string{{"", "a", "a"}}, Output: ",a,a\n"},
	{Input: [][]string{{"a", "", ""}}, Output: "a,,\n"},
	{Input: [][]string{{"a", "", "a"}}, Output: "a,,a\n"},
	{Input: [][]string{{"a", "a", ""}}, Output: "a,a,\n"},
	{Input: [][]string{{"a", "a", "a"}}, Output: "a,a,a\n"},
	{Input: [][]string{{`\.`}}, Output: "\"\\.\"\n"},
}

func TestWrite(t *testing.T) {
	for n, tt := range writeTests {
		b := &bytes.Buffer{}
		f := NewWriter(b)
		f.UseCRLF = tt.UseCRLF
		err := f.WriteAll(tt.Input)
		if err != nil {
			t.Errorf("Unexpected error: %s\n", err)
		}
		out := b.String()
		if out != tt.Output {
			t.Errorf("#%d: out=%q want %q", n, out, tt.Output)
		}
	}
}

// This tests round tripping through WriteAny() and ReadAny().
var writeAnyTests = []struct {
	Input          [][]interface{}
	Output         string
	UseCRLF        bool
	TestEncodeOnly bool
}{
	// Simple strings are written without quoting.
	{Input: [][]interface{}{{"a", "b", "c"}}, Output: "a,b,c\n"},

	// Integers and floats are written without quotes.
	{Input: [][]interface{}{{1, 2, 3.1}}, Output: "1,2,3.1\n"},

	// Strings with line feeds are quoted.
	{Input: [][]interface{}{{"a\nb", 2, "c\nd"}},
		Output: "\"a\nb\",2,\"c\nd\"\n"},

	// Known bug: Strings with \r\n get converted to \n because
	// csv module converts \r\n to \n even within a quoted item.

	//	{Input: [][]interface{}{{"a\nb", 2, "c\r\nd"}},
	//		Output: "\"a\nb\",2,\"c\r\nd\"\n"},

	// Raw Bytes are converted to base64
	{Input: [][]interface{}{{[]byte("hello")}}, Output: "base64:aGVsbG8=\n"},

	// Regular strings which happen to look like base64 encoded
	// data are not accidentally decoded.
	{Input: [][]interface{}{{"base64:aGVsbG8="}}, Output: "\" base64:aGVsbG8=\"\n"},
	{Input: [][]interface{}{{" base64:aGVsbG8="}}, Output: "\"  base64:aGVsbG8=\"\n"},

	// Encode ints, floats without quotes and strings with quotes.
	{Input: [][]interface{}{{1, "2", 4.5, "abc"}},
		Output: "1,\" 2\",4.5,abc\n"},

	// Ints, maps and strings.
	{Input: [][]interface{}{{1, ordereddict.NewDict().Set("foo", "bar"), "hello"}},
		Output: "1,\"{\n \"\"foo\"\": \"\"bar\"\"\n}\",hello\n",
	},

	// OSPath is encoded correctly.
	{Input: [][]interface{}{{accessors.MustNewGenericOSPath("/bin/ls")}},
		TestEncodeOnly: true,
		Output:         "/bin/ls\n"},
}

func TestWriteAny(t *testing.T) {
	for n, tt := range writeAnyTests {
		b := &bytes.Buffer{}
		f := NewWriter(b)
		f.UseCRLF = tt.UseCRLF
		for _, item := range tt.Input {
			err := f.WriteAny(item, json.DefaultEncOpts())
			if err != nil {
				t.Errorf("Unexpected error: %s\n", err)
			}
		}
		f.Flush()

		out := b.String()

		if out != tt.Output {
			t.Errorf("#%d: out=%q want %q", n, out, tt.Output)
		}

		if tt.TestEncodeOnly {
			continue
		}

		rows := [][]interface{}{}

		r, err := NewReader(strings.NewReader(out))
		assert.NoError(t, err)

		for {
			row, err := r.ReadAny()
			if err != nil {
				break
			}
			rows = append(rows, row)
		}

		if !reflect.DeepEqual(rows, tt.Input) {
			utils.Debug(rows)
			utils.Debug(tt.Input)
			t.Errorf("Unable to decode: Got %v expected %v", rows, tt.Input)
		}
	}
}

type errorWriter struct{}

func (e errorWriter) Write(b []byte) (int, error) {
	return 0, errors.New("Test")
}

func TestError(t *testing.T) {
	b := &bytes.Buffer{}
	f := NewWriter(b)
	f.Write([]string{"abc"})
	f.Flush()
	err := f.Error()

	if err != nil {
		t.Errorf("Unexpected error: %s\n", err)
	}

	f = NewWriter(errorWriter{})
	f.Write([]string{"abc"})
	f.Flush()
	err = f.Error()

	if err == nil {
		t.Error("Error should not be nil")
	}
}
