// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package csv

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func TestRead(t *testing.T) {
	tests := []struct {
		Name   string
		Input  string
		Output [][]string
		Error  error

		// These fields are copied into the Reader
		Comma                rune
		Comment              rune
		UseFieldsPerRecord   bool // false (default) means FieldsPerRecord is -1
		FieldsPerRecord      int
		LazyQuotes           bool
		TrimLeadingSpace     bool
		ReuseRecord          bool
		RequireLineSeperator bool

		// What should the byte offset be reported as?
		ByteOffset int64
	}{{
		Name:       "Simple",
		Input:      "a,b,c\n",
		Output:     [][]string{{"a", "b", "c"}},
		ByteOffset: 6,
	}, {
		Name:       "CRLF",
		Input:      "a,b\r\nc,d\r\n",
		Output:     [][]string{{"a", "b"}, {"c", "d"}},
		ByteOffset: 10,
	}, {
		Name:       "BareCR",
		Input:      "a,b\rc,d\r\n",
		Output:     [][]string{{"a", "b\rc", "d"}},
		ByteOffset: 9,
	}, {
		Name: "RFC4180test",
		Input: `#field1,field2,field3
"aaa","bb
b","ccc"
"a,a","b""bb","ccc"
zzz,yyy,xxx
`,
		Output: [][]string{
			{"#field1", "field2", "field3"},
			{"aaa", "bb\nb", "ccc"},
			{"a,a", `b"bb`, "ccc"},
			{"zzz", "yyy", "xxx"},
		},
		ByteOffset:         73,
		UseFieldsPerRecord: true,
		FieldsPerRecord:    0,
	}, {
		Name:                 "TruncatedEOF",
		Input:                "1,2,3\na,b",
		ByteOffset:           6,
		RequireLineSeperator: true,
		Error: &ParseError{StartLine: 2, Line: 2, Column: 0,
			Err: ErrNoLineFeed},
	}, {
		Name:                 "TruncatedMultilineEOF",
		Input:                "1,2,3\na,123,234",
		ByteOffset:           6,
		RequireLineSeperator: true,
		Error: &ParseError{StartLine: 2, Line: 2, Column: 0,
			Err: ErrNoLineFeed},
	}, {
		Name:                 "NoEOLTest",
		Input:                "a,b,c",
		ByteOffset:           0,
		RequireLineSeperator: true,
		Error: &ParseError{StartLine: 1, Line: 1, Column: 0,
			Err: ErrNoLineFeed},
	}, {
		Name:       "Semicolon",
		Input:      "a;b;c\n",
		Output:     [][]string{{"a", "b", "c"}},
		Comma:      ';',
		ByteOffset: 6,
	}, {
		Name: "MultiLine",
		Input: `"two
line","one line","three
line
field"`,
		Output:     [][]string{{"two\nline", "one line", "three\nline\nfield"}},
		ByteOffset: 40,
	}, {
		Name:  "BlankLine",
		Input: "a,b,c\n\nd,e,f\n\n",
		Output: [][]string{
			{"a", "b", "c"},
			{"d", "e", "f"},
		},
		ByteOffset: 13,
	}, {
		Name:  "BlankLineFieldCount",
		Input: "a,b,c\n\nd,e,f\n\n",
		Output: [][]string{
			{"a", "b", "c"},
			{"d", "e", "f"},
		},
		UseFieldsPerRecord: true,
		FieldsPerRecord:    0,
		ByteOffset:         13,
	}, {
		Name:             "TrimSpace",
		Input:            " a,  b,   c\n",
		Output:           [][]string{{"a", "b", "c"}},
		TrimLeadingSpace: true,
		ByteOffset:       12,
	}, {
		Name:       "LeadingSpace",
		Input:      " a,  b,   c\n",
		Output:     [][]string{{" a", "  b", "   c"}},
		ByteOffset: 12,
	}, {
		Name:       "Comment",
		Input:      "#1,2,3\na,b,c\n#comment",
		Output:     [][]string{{"a", "b", "c"}},
		Comment:    '#',
		ByteOffset: 13,
	}, {
		Name:       "NoComment",
		Input:      "#1,2,3\na,b,c",
		Output:     [][]string{{"#1", "2", "3"}, {"a", "b", "c"}},
		ByteOffset: 12,
	}, {
		Name:       "LazyQuotes",
		Input:      `a "word","1"2",a","b`,
		Output:     [][]string{{`a "word"`, `1"2`, `a"`, `b`}},
		LazyQuotes: true,
		ByteOffset: 20,
	}, {
		Name:       "BareQuotes",
		Input:      `a "word","1"2",a"`,
		Output:     [][]string{{`a "word"`, `1"2`, `a"`}},
		LazyQuotes: true,
		ByteOffset: 17,
	}, {
		Name:       "BareDoubleQuotes",
		Input:      `a""b,c`,
		Output:     [][]string{{`a""b`, `c`}},
		LazyQuotes: true,
		ByteOffset: 6,
	}, {
		Name: "BadDoubleQuotes",
		Input: `1,2,3
a""b,c`,
		Error:      &ParseError{StartLine: 2, Line: 2, Column: 1, Err: ErrBareQuote},
		ByteOffset: 6,
	}, {
		Name:             "TrimQuote",
		Input:            ` "a"," b",c`,
		Output:           [][]string{{"a", " b", "c"}},
		TrimLeadingSpace: true,
		ByteOffset:       11,
	}, {
		Name: "BadBareQuote",
		Input: `1,2,3
a "word","b"`,
		Error:      &ParseError{StartLine: 2, Line: 2, Column: 2, Err: ErrBareQuote},
		ByteOffset: 6,
	}, {
		Name: "BadTrailingQuote",
		Input: `1,2,3
"a word",b"`,
		Error:      &ParseError{StartLine: 2, Line: 2, Column: 10, Err: ErrBareQuote},
		ByteOffset: 6,
	}, {
		Name:       "ExtraneousQuote",
		Input:      `"a "word","b"`,
		Error:      &ParseError{StartLine: 1, Line: 1, Column: 3, Err: ErrQuote},
		ByteOffset: 0,
	}, {
		Name:       "FieldCount",
		Input:      "a,b,c\nd,e",
		Output:     [][]string{{"a", "b", "c"}, {"d", "e"}},
		ByteOffset: 9,
	}, {
		Name:       "TrailingCommaEOF",
		Input:      "a,b,c,",
		Output:     [][]string{{"a", "b", "c", ""}},
		ByteOffset: 6,
	}, {
		Name:       "TrailingCommaEOL",
		Input:      "a,b,c,\n",
		Output:     [][]string{{"a", "b", "c", ""}},
		ByteOffset: 7,
	}, {
		Name:             "TrailingCommaSpaceEOF",
		Input:            "a,b,c, ",
		Output:           [][]string{{"a", "b", "c", ""}},
		TrimLeadingSpace: true,
		ByteOffset:       7,
	}, {
		Name:             "TrailingCommaSpaceEOL",
		Input:            "a,b,c, \n",
		Output:           [][]string{{"a", "b", "c", ""}},
		TrimLeadingSpace: true,
		ByteOffset:       8,
	}, {
		Name:             "TrailingCommaLine3",
		Input:            "a,b,c\nd,e,f\ng,hi,",
		Output:           [][]string{{"a", "b", "c"}, {"d", "e", "f"}, {"g", "hi", ""}},
		TrimLeadingSpace: true,
		ByteOffset:       17,
	}, {
		Name:       "NotTrailingComma3",
		Input:      "a,b,c, \n",
		Output:     [][]string{{"a", "b", "c", " "}},
		ByteOffset: 8,
	}, {
		Name: "CommaFieldTest",
		Input: `x,y,z,w
x,y,z,
x,y,,
x,,,
,,,
"x","y","z","w"
"x","y","z",""
"x","y","",""
"x","","",""
"","","",""
`,
		Output: [][]string{
			{"x", "y", "z", "w"},
			{"x", "y", "z", ""},
			{"x", "y", "", ""},
			{"x", "", "", ""},
			{"", "", "", ""},
			{"x", "y", "z", "w"},
			{"x", "y", "z", ""},
			{"x", "y", "", ""},
			{"x", "", "", ""},
			{"", "", "", ""},
		},
		ByteOffset: 100,
	}, {
		Name:  "TrailingCommaIneffective1",
		Input: "a,b,\nc,d,e",
		Output: [][]string{
			{"a", "b", ""},
			{"c", "d", "e"},
		},
		TrimLeadingSpace: true,
		ByteOffset:       10,
	}, {
		Name:  "ReadAllReuseRecord",
		Input: "a,b\nc,d",
		Output: [][]string{
			{"a", "b"},
			{"c", "d"},
		},
		ReuseRecord: true,
		ByteOffset:  7,
	}, {
		Name:       "StartLine1", // Issue 19019
		Input:      "1,2,3\na,\"b\nc\"d,e",
		Error:      &ParseError{StartLine: 2, Line: 3, Column: 1, Err: ErrQuote},
		ByteOffset: 6,
	}, {
		Name:       "StartLine2",
		Input:      "a,b\n\"d\n\n,e",
		Error:      &ParseError{StartLine: 2, Line: 5, Column: 0, Err: ErrQuote},
		ByteOffset: 4,
	}, {
		Name:  "CRLFInQuotedField", // Issue 21201
		Input: "A,\"Hello\r\nHi\",B\r\n",
		Output: [][]string{
			{"A", "Hello\nHi", "B"},
		},
		ByteOffset: 17,
	}, {
		Name:       "BinaryBlobField", // Issue 19410
		Input:      "x09\x41\xb4\x1c,aktau",
		Output:     [][]string{{"x09A\xb4\x1c", "aktau"}},
		ByteOffset: 12,
	}, {
		Name:       "TrailingCR",
		Input:      "field1,field2\r",
		Output:     [][]string{{"field1", "field2"}},
		ByteOffset: 14,
	}, {
		Name:       "QuotedTrailingCR",
		Input:      "\"field\"\r",
		Output:     [][]string{{"field"}},
		ByteOffset: 8,
	}, {
		Name:       "IncompleteLine",
		Input:      "a,b,c,\"d\na,b,c,d",
		Error:      &ParseError{StartLine: 1, Line: 3, Column: 0, Err: ErrQuote},
		ByteOffset: 0,
	}, {
		Name:       "QuotedTrailingCRCR",
		Input:      "\"field\"\r\r",
		Error:      &ParseError{StartLine: 1, Line: 1, Column: 6, Err: ErrQuote},
		ByteOffset: 0,
	}, {
		Name:       "FieldCR",
		Input:      "field\rfield\r",
		Output:     [][]string{{"field\rfield"}},
		ByteOffset: 12,
	}, {
		Name:       "FieldCRCR",
		Input:      "field\r\rfield\r\r",
		Output:     [][]string{{"field\r\rfield\r"}},
		ByteOffset: 14,
	}, {
		Name:       "FieldCRCRLF",
		Input:      "field\r\r\nfield\r\r\n",
		Output:     [][]string{{"field\r"}, {"field\r"}},
		ByteOffset: 16,
	}, {
		Name:       "FieldCRCRLFCR",
		Input:      "field\r\r\n\rfield\r\r\n\r",
		Output:     [][]string{{"field\r"}, {"\rfield\r"}},
		ByteOffset: 17,
	}, {
		Name:       "FieldCRCRLFCRCR",
		Input:      "field\r\r\n\r\rfield\r\r\n\r\r",
		Output:     [][]string{{"field\r"}, {"\r\rfield\r"}, {"\r"}},
		ByteOffset: 20,
	}, {
		Name:  "MultiFieldCRCRLFCRCR",
		Input: "field1,field2\r\r\n\r\rfield1,field2\r\r\n\r\r,",
		Output: [][]string{
			{"field1", "field2\r"},
			{"\r\rfield1", "field2\r"},
			{"\r\r", ""},
		},
		ByteOffset: 37,
	}, {
		Name:             "NonASCIICommaAndComment",
		Input:            "a£b,c£ \td,e\n€ comment\n",
		Output:           [][]string{{"a", "b,c", "d,e"}},
		TrimLeadingSpace: true,
		Comma:            '£',
		Comment:          '€',
		ByteOffset:       14,
	}, {
		Name:       "NonASCIICommaAndCommentWithQuotes",
		Input:      "a€\"  b,\"€ c\nλ comment\n",
		Output:     [][]string{{"a", "  b,", " c"}},
		Comma:      '€',
		Comment:    'λ',
		ByteOffset: 16,
	}, {
		// λ and θ start with the same byte.
		// This tests that the parser doesn't confuse such characters.
		Name:       "NonASCIICommaConfusion",
		Input:      "\"abθcd\"λefθgh",
		Output:     [][]string{{"abθcd", "efθgh"}},
		Comma:      'λ',
		Comment:    '€',
		ByteOffset: 16,
	}, {
		Name:       "NonASCIICommentConfusion",
		Input:      "λ\nλ\nθ\nλ\n",
		Output:     [][]string{{"λ"}, {"λ"}, {"λ"}},
		Comment:    'θ',
		ByteOffset: 12,
	}, {
		Name:       "QuotedFieldMultipleLF",
		Input:      "\"\n\n\n\n\"",
		Output:     [][]string{{"\n\n\n\n"}},
		ByteOffset: 6,
	}, {
		Name:       "MultipleCRLF",
		Input:      "\r\n\r\n\r\n\r\n",
		ByteOffset: 0,
	}, {
		// The implementation may read each line in several chunks if it doesn't fit entirely
		// in the read buffer, so we should test the code to handle that condition.
		Name:       "HugeLines",
		Input:      strings.Repeat("#ignore\n", 10000) + strings.Repeat("@", 5000) + "," + strings.Repeat("*", 5000),
		Output:     [][]string{{strings.Repeat("@", 5000), strings.Repeat("*", 5000)}},
		Comment:    '#',
		ByteOffset: 90001,
	}, {
		Name:       "QuoteWithTrailingCRLF",
		Input:      "\"foo\"bar\"\r\n",
		Error:      &ParseError{StartLine: 1, Line: 1, Column: 4, Err: ErrQuote},
		ByteOffset: 0,
	}, {
		Name:       "LazyQuoteWithTrailingCRLF",
		Input:      "\"foo\"bar\"\r\n",
		Output:     [][]string{{`foo"bar`}},
		LazyQuotes: true,
		ByteOffset: 11,
	}, {
		Name:       "DoubleQuoteWithTrailingCRLF",
		Input:      "\"foo\"\"bar\"\r\n",
		Output:     [][]string{{`foo"bar`}},
		ByteOffset: 12,
	}, {
		Name:       "EvenQuotes",
		Input:      `""""""""`,
		Output:     [][]string{{`"""`}},
		ByteOffset: 8,
	}, {
		Name:       "OddQuotes",
		Input:      `"""""""`,
		Error:      &ParseError{StartLine: 1, Line: 1, Column: 7, Err: ErrQuote},
		ByteOffset: 0,
	}, {
		Name:       "LazyOddQuotes",
		Input:      `"""""""`,
		Output:     [][]string{{`"""`}},
		LazyQuotes: true,
		ByteOffset: 7,
	}, {
		Name:  "BadComma1",
		Comma: '\n',
		Error: errInvalidDelim,
	}, {
		Name:  "BadComma2",
		Comma: '\r',
		Error: errInvalidDelim,
	}, {
		Name:  "BadComma3",
		Comma: utf8.RuneError,
		Error: errInvalidDelim,
	}, {
		Name:    "BadComment1",
		Comment: '\n',
		Error:   errInvalidDelim,
	}, {
		Name:    "BadComment2",
		Comment: '\r',
		Error:   errInvalidDelim,
	}, {
		Name:    "BadComment3",
		Comment: utf8.RuneError,
		Error:   errInvalidDelim,
	}, {
		Name:    "BadCommaComment",
		Comma:   'X',
		Comment: 'X',
		Error:   errInvalidDelim,
	}}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			r, err := NewReader(strings.NewReader(tt.Input))
			assert.NoError(t, err)

			if tt.Comma != 0 {
				r.Comma = tt.Comma
			}
			r.Comment = tt.Comment
			if tt.UseFieldsPerRecord {
				r.FieldsPerRecord = tt.FieldsPerRecord
			} else {
				r.FieldsPerRecord = -1
			}
			r.LazyQuotes = tt.LazyQuotes
			r.TrimLeadingSpace = tt.TrimLeadingSpace
			r.ReuseRecord = tt.ReuseRecord

			if tt.RequireLineSeperator {
				r.RequireLineSeperator = true
			}

			out, err := r.ReadAll()
			if !reflect.DeepEqual(err, tt.Error) {
				t.Errorf("ReadAll() error:\ngot  %v\nwant %v", err, tt.Error)
			} else if !reflect.DeepEqual(out, tt.Output) {
				t.Errorf("ReadAll() output:\ngot  %q\nwant %q", out, tt.Output)
			}

			if r.ByteOffset != tt.ByteOffset {
				t.Errorf("ByteOffset error:\ngot  %v\nwant %v",
					r.ByteOffset, tt.ByteOffset)
			}
		})
	}
}
