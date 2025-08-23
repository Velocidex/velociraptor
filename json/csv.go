// Optimized function to convert a JSONL sequence into CSV and
// modified JSONL output. This optimization is used for exporting
// containers quickly.
package json

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
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
	extra_data *ordereddict.Dict) {

	//var columns []string
	var p fastjson.Parser
	csv_encoder := NewCSVEncoder(extra_data)
	var extra_keys []string
	var extra_value [][]byte

	if extra_data != nil {
		for _, i := range extra_data.Items() {
			v, err := json.Marshal(i.Value)
			if err != nil {
				continue
			}
			extra_keys = append(extra_keys, i.Key)
			extra_value = append(extra_value, v)
		}
	}

	arena := &fastjson.Arena{}

	for serialized := range json_in {
		if len(serialized) == 0 {
			continue
		}

		// Line feed if needed.
		if serialized[len(serialized)-1] != '\n' {
			serialized = append(serialized, '\n')
		}

		// In the special case where we do not need to modify the json
		// or convert it to csv then we can skip parsing it
		// altogether.
		if extra_data == nil && jsonl_out != nil && csv_out == nil {
			_, _ = jsonl_out.Write(serialized)
			continue
		}

		v, err := p.Parse(string(serialized))
		if err != nil {
			continue
		}

		obj, err := v.Object()
		if err != nil {
			continue
		}

		if jsonl_out != nil {
			// If we dont need to add any columns we just copy the
			// original JSONL without needing to encode it.
			if extra_data == nil {
				_, _ = jsonl_out.Write(serialized)
			} else {
				_, _ = jsonl_out.Write(
					writeJsonObject(arena, obj, extra_keys, extra_value))
			}
		}

		if csv_out != nil {
			_, _ = csv_out.Write(csv_encoder.Encode(obj))
		}
	}
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
				self.row[idx] = string(b)
			}

			// Nulls should be written as a empty strings
		case fastjson.TypeNull:
			self.row[idx] = ""

			// Everything else will be JSON encoded.
		default:
			self.row[idx] = string(v.MarshalTo(nil))
		}

	})

	for i, k := range self.extra_keys {
		idx, pres := self.column_idx[k]
		if !pres {
			continue
		}

		self.row[idx] = self.extra_values[i]
	}

	_ = self.writer.Write(self.row)
	self.writer.Flush()

	result := self.buf.Bytes()
	self.buf.Reset()

	return result
}

func writeJsonObject(
	arena *fastjson.Arena,
	obj *fastjson.Object,
	extra_keys []string, extra_values [][]byte) []byte {
	buf := make([]byte, 0)
	buf = append(buf, '{')

	obj.Visit(func(key []byte, v *fastjson.Value) {
		// Add key
		key_v := arena.NewStringBytes(key)
		buf = append(buf, key_v.MarshalTo(nil)...)
		buf = append(buf, ':')
		buf = append(buf, v.MarshalTo(nil)...)
		buf = append(buf, ',')
	})

	if len(extra_keys) == len(extra_values) {
		for i := 0; i < len(extra_values); i++ {
			buf = append(buf, '"')
			buf = append(buf, extra_keys[i]...)
			buf = append(buf, '"')
			buf = append(buf, ':')
			buf = append(buf, extra_values[i]...)
			buf = append(buf, ',')
		}
	}

	if len(buf) == 0 {
		buf = append(buf, '{', '}')
		return buf
	}

	buf[len(buf)-1] = '}'
	buf = append(buf, '\n')

	return buf
}
