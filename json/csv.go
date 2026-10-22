// Optimized function to convert a JSONL sequence into CSV and
// modified JSONL output. This optimization is used for exporting
// containers quickly.
package json

import (
	"bytes"
	"encoding/csv"
	"io"

	"github.com/Velocidex/ordereddict"
	"github.com/valyala/fastjson"
)

// Reads raw JSON object, modifies it and writes it as a raw line to
// either/both json or csv consumer.
func ConvertJSONL(
	json_in <-chan []byte,

	// If either of these is nil we skip writing to it.
	jsonl_out io.Writer,
	csv_out io.Writer,
	extra_data *ordereddict.Dict) error {

	var p fastjson.Parser
	csv_encoder := NewCSVEncoder(extra_data)
	extra, err := encodeExtraFields(extra_data)
	if err != nil {
		return err
	}

	var buf []byte

	for serialized := range json_in {
		if len(serialized) == 0 {
			continue
		}

		// Line feed if needed.
		if serialized[len(serialized)-1] != '\n' {
			serialized = append(serialized, '\n')
		}

		var obj *fastjson.Object
		if csv_out != nil {
			v, err := p.Parse(string(serialized))
			if err != nil {
				continue
			}

			obj, err = v.Object()
			if err != nil {
				continue
			}
		}

		if jsonl_out != nil {
			// If we don't need to add any columns we just copy the
			// original JSONL without needing to encode it.
			if extra_data == nil {
				_, _ = jsonl_out.Write(serialized)
			} else {
				buf = appendJsonlExtra(buf[:0], serialized, extra)
				_, _ = jsonl_out.Write(buf)
			}
		}

		if csv_out != nil {
			_, _ = csv_out.Write(csv_encoder.Encode(obj))
		}
	}

	return nil
}

// Builds `"FlowId":"F.1","ClientId":"C.1"` once, reused for every line.
func encodeExtraFields(extra_data *ordereddict.Dict) ([]byte, error) {
	serialized, err := Marshal(extra_data)
	if err != nil {
		return nil, err
	}

	if len(serialized) < 2 {
		return nil, nil
	}

	// Trim the opening and closing {}
	return serialized[1 : len(serialized)-1], nil
}

const whiteSpace = " \t\r\n"

// Turns `{"a":1}` into `{"a":1,<extra>}` without parsing; non-objects are dropped.
func appendJsonlExtra(dst, line, extra []byte) []byte {
	line = bytes.TrimRight(line, whiteSpace)

	// Check the line for sanity
	if len(line) < 2 || line[0] != '{' || line[len(line)-1] != '}' {
		return nil
	}

	// Line should not contain any \r or \n as it should be a fixed
	// jsonl line.
	if bytes.ContainsAny(line, "\n\r") {
		return nil
	}

	// Drop the last } from the line
	line = line[:len(line)-1]

	dst = append(dst, bytes.TrimRight(line, whiteSpace)...)

	// No comma after `{` for an empty object.
	if len(extra) > 0 &&
		!bytes.HasSuffix(dst, []byte{'{'}) {
		dst = append(dst, ',')
	}
	dst = append(dst, extra...)
	return append(dst, '}', '\n')
}

type CSVEncoder struct {
	writer     *csv.Writer
	columns    []string
	column_idx map[string]int

	extra_keys   []string
	extra_values []string

	row []string
	buf bytes.Buffer
}

func NewCSVEncoder(extra_data *ordereddict.Dict) *CSVEncoder {
	self := &CSVEncoder{
		column_idx: make(map[string]int),
	}
	self.writer = csv.NewWriter(&self.buf)

	if extra_data != nil {
		for _, i := range extra_data.Items() {
			self.extra_keys = append(self.extra_keys, i.Key)
			self.extra_values = append(self.extra_values,
				AnyToString(i.Value, DefaultEncOpts()))
		}
	}

	return self
}

func (self *CSVEncoder) Encode(obj *fastjson.Object) []byte {
	// Figure out the columns by the first object
	if len(self.columns) == 0 {
		obj.Visit(func(key []byte, v *fastjson.Value) {
			self.columns = append(self.columns, string(key))
			self.column_idx[string(key)] = len(self.column_idx)
			self.row = append(self.row, "")
		})

		for i := 0; i < len(self.extra_keys); i++ {
			self.columns = append(self.columns, self.extra_keys[i])
			self.column_idx[self.extra_keys[i]] = len(self.column_idx)
			self.row = append(self.row, "")
		}

		// Encode the headers
		_ = self.writer.Write(self.columns)
	}

	if len(self.columns) == 0 {
		return nil
	}

	// Reset the row
	for i := 0; i < len(self.row); i++ {
		self.row[i] = ""
	}

	// Copy the fields into the row
	obj.Visit(func(key []byte, v *fastjson.Value) {
		idx, pres := self.column_idx[string(key)]
		if !pres {
			return
		}

		// Encode the value into Velociraptor's CSV as described in
		// file_store/csv/doc.go
		switch v.Type() {

		// Write strings directly without escaping - csv will escape
		// if needed.
		case fastjson.TypeString:
			// It is already a string
			b, err := v.StringBytes()
			if err == nil {
				self.row[idx] = sanitizeExcelFormulas(string(b))
			}

			// Nulls should be written as a empty strings
		case fastjson.TypeNull:
			self.row[idx] = ""

			// Everything else will be JSON encoded.
		default:
			self.row[idx] = sanitizeExcelFormulas(string(v.MarshalTo(nil)))
		}

	})

	for i, k := range self.extra_keys {
		idx, pres := self.column_idx[k]
		if !pres {
			continue
		}

		self.row[idx] = sanitizeExcelFormulas(self.extra_values[i])
	}

	_ = self.writer.Write(self.row)
	self.writer.Flush()

	result := self.buf.Bytes()
	self.buf.Reset()

	return result
}

// https://owasp.org/www-community/attacks/CSV_Injection
func sanitizeExcelFormulas(in string) string {
	if len(in) > 0 {
		switch in[0] {
		case '=', '+', '@', '\x09', '\x0d', '\x0a':
			return "\t" + in
		}
	}
	return in
}
