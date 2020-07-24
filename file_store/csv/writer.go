// Copyright 2011 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package csv

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"www.velocidex.com/golang/velociraptor/json"

	"www.velocidex.com/golang/vfilter"
)

var (
	number_regex = regexp.MustCompile(
		`^(?i)(?P<Number>[-+]?\d*\.?\d+([eE][-+]?\d+)?)$`)

	// Strings that look like this will be escaped because they
	// might be confused with other things.
	protected_prefix = regexp.MustCompile(
		`(?i)^( |\{|\[|true|false|[+-]?inf|base64:)`)
)

// A Writer writes records to a CSV encoded file.
//
// As returned by NewWriter, a Writer writes records terminated by a
// newline and uses ',' as the field delimiter. The exported fields can be
// changed to customize the details before the first call to Write or WriteAll.
//
// Comma is the field delimiter.
//
// If UseCRLF is true, the Writer ends each output line with \r\n instead of \n.
type Writer struct {
	Comma   rune // Field delimiter (set to ',' by NewWriter)
	UseCRLF bool // True to use \r\n as the line terminator
	w       *bufio.Writer
}

// NewWriter returns a new Writer that writes to w.
func NewWriter(w io.Writer) *Writer {
	return &Writer{
		Comma: ',',
		w:     bufio.NewWriter(w),
	}
}

func AnyToString(item vfilter.Any) string {
	value := ""

	switch t := item.(type) {
	case float32:
		value = strconv.FormatFloat(float64(t), 'f', -1, 64)

	case float64:
		value = strconv.FormatFloat(t, 'f', -1, 64)

	case time.Time:
		value = t.Format(time.RFC3339Nano)

	case int, int16, int32, int64, uint16, uint32, uint64, bool:
		value = fmt.Sprintf("%v", item)

	case []byte:
		value = "base64:" + base64.StdEncoding.EncodeToString(t)

	case string:
		// If the string looks like a number we encode
		// it as a json object. This will ensure that
		// the reader does not get confused between
		// strings which look like a number and
		// numbers.
		if number_regex.MatchString(t) ||
			protected_prefix.MatchString(t) {
			value = " " + t
		} else {
			value = t
		}

	default:
		serialized, err := json.MarshalIndent(item)
		if err != nil {
			return ""
		}

		if len(serialized) > 0 && (serialized[0] == '{' ||
			serialized[0] == '[') {
			value = string(serialized)
		}
	}

	return value
}

func (w *Writer) WriteAny(record []interface{}) error {
	row := []string{}

	for _, item := range record {
		row = append(row, AnyToString(item))
	}

	return w.Write(row)
}

// Writer writes a single CSV record to w along with any necessary quoting.
// A record is a slice of strings with each string being one field.
func (w *Writer) Write(record []string) error {
	if !validDelim(w.Comma) {
		return errInvalidDelim
	}

	for n, field := range record {
		if n > 0 {
			if _, err := w.w.WriteRune(w.Comma); err != nil {
				return err
			}
		}

		// If we don't have to have a quoted field then just
		// write out the field and continue to the next field.
		if !w.fieldNeedsQuotes(field) {
			if _, err := w.w.WriteString(field); err != nil {
				return err
			}
			continue
		}
		if err := w.w.WriteByte('"'); err != nil {
			return err
		}

		for _, r1 := range field {
			var err error
			switch r1 {
			case '"':
				_, err = w.w.WriteString(`""`)
			case '\r':
				if !w.UseCRLF {
					err = w.w.WriteByte('\r')
				}
			case '\n':
				if w.UseCRLF {
					_, err = w.w.WriteString("\r\n")
				} else {
					err = w.w.WriteByte('\n')
				}
			default:
				_, err = w.w.WriteRune(r1)
			}
			if err != nil {
				return err
			}
		}

		if err := w.w.WriteByte('"'); err != nil {
			return err
		}
	}
	var err error
	if w.UseCRLF {
		_, err = w.w.WriteString("\r\n")
	} else {
		err = w.w.WriteByte('\n')
	}
	return err
}

// Flush writes any buffered data to the underlying io.Writer.
// To check if an error occurred during the Flush, call Error.
func (w *Writer) Flush() {
	w.w.Flush()
}

// Error reports any error that has occurred during a previous Write or Flush.
func (w *Writer) Error() error {
	_, err := w.w.Write(nil)
	return err
}

// WriteAll writes multiple CSV records to w using Write and then calls Flush.
func (w *Writer) WriteAll(records [][]string) error {
	for _, record := range records {
		err := w.Write(record)
		if err != nil {
			return err
		}
	}
	return w.w.Flush()
}

// fieldNeedsQuotes reports whether our field must be enclosed in quotes.
// Fields with a Comma, fields with a quote or newline, and
// fields which start with a space must be enclosed in quotes.
// We used to quote empty strings, but we do not anymore (as of Go 1.4).
// The two representations should be equivalent, but Postgres distinguishes
// quoted vs non-quoted empty string during database imports, and it has
// an option to force the quoted behavior for non-quoted CSV but it has
// no option to force the non-quoted behavior for quoted CSV, making
// CSV with quoted empty strings strictly less useful.
// Not quoting the empty string also makes this package match the behavior
// of Microsoft Excel and Google Drive.
// For Postgres, quote the data terminating string `\.`.
func (w *Writer) fieldNeedsQuotes(field string) bool {
	if field == "" {
		return false
	}
	if field == `\.` || strings.ContainsRune(field, w.Comma) || strings.ContainsAny(field, "\"\r\n") {
		return true
	}

	r1, _ := utf8.DecodeRuneInString(field)
	return unicode.IsSpace(r1)
}
