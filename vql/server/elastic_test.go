package server

import (
	"testing"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/stretchr/testify/assert"
)

func TestDetermineTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		input    *ordereddict.Dict
		expected time.Time
	}{
		{
			name:     "No timestamp fields",
			input:    ordereddict.NewDict(),
			expected: time.Now().UTC(), // Approximate check below
		},
		{
			name: "RFC3339 String",
			input: ordereddict.NewDict().
				Set("timestamp", "2023-10-27T10:00:00Z"),
			expected: time.Date(2023, 10, 27, 10, 0, 0, 0, time.UTC),
		},
		{
			name: "RFC3339Nano String",
			input: ordereddict.NewDict().
				Set("time", "2023-10-27T10:00:00.123456Z"),
			expected: time.Date(2023, 10, 27, 10, 0, 0, 123456000, time.UTC),
		},
		{
			name: "Numeric Seconds",
			input: ordereddict.NewDict().
				Set("@timestamp", float64(1698393600)), // 2023-10-27 08:00:00 UTC
			expected: time.Date(2023, 10, 27, 8, 0, 0, 0, time.UTC),
		},
		{
			name: "Numeric Milliseconds",
			input: ordereddict.NewDict().
				Set("timestamp", int64(1698393600000)),
			expected: time.Date(2023, 10, 27, 8, 0, 0, 0, time.UTC),
		},
		{
			name: "Numeric Microseconds",
			input: ordereddict.NewDict().
				Set("time", int64(1698393600000000)),
			expected: time.Date(2023, 10, 27, 8, 0, 0, 0, time.UTC),
		},
		{
			name: "Numeric Nanoseconds",
			input: ordereddict.NewDict().
				Set("timestamp", int64(1698393600000000000)),
			expected: time.Date(2023, 10, 27, 8, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineTimestamp(tt.input)
			if tt.name == "No timestamp fields" {
				assert.WithinDuration(t, tt.expected, result, time.Second)
			} else {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestDetermineTimestampDescValue(t *testing.T) {
	tests := []struct {
		name     string
		input    *ordereddict.Dict
		expected string
	}{
		{
			name:     "Empty",
			input:    ordereddict.NewDict(),
			expected: "Velociraptor Event",
		},
		{
			name: "Event Field",
			input: ordereddict.NewDict().
				Set("event", "Process Creation"),
			expected: "Process Creation",
		},
		{
			name: "Operation Field",
			input: ordereddict.NewDict().
				Set("operation", "File Read"),
			expected: "File Read",
		},
		{
			name: "Both Fields (Event Priority)",
			input: ordereddict.NewDict().
				Set("event", "Process Creation").
				Set("operation", "File Read"),
			expected: "Process Creation",
		},
		{
			name: "Empty Event String",
			input: ordereddict.NewDict().
				Set("event", "   "),
			expected: "Velociraptor Event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := determineTimestampDescValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestApplyTimesketchEnrichment(t *testing.T) {
	// Base time for testing
	refTime := time.Date(2023, 10, 27, 10, 0, 0, 0, time.UTC)
	refTimeStr := refTime.Format(time.RFC3339)
	refTimeNanoStr := refTime.Format(time.RFC3339Nano)
	refTimeMicro := refTime.UnixNano() / int64(time.Microsecond)

	tests := []struct {
		name   string
		input  *ordereddict.Dict
		verify func(*testing.T, *ordereddict.Dict)
	}{
		{
			name: "Basic Enrichment",
			input: ordereddict.NewDict().
				Set("foo", "bar").
				Set("timestamp", refTimeStr),
			verify: func(t *testing.T, d *ordereddict.Dict) {
				// Check datetime
				val, _ := d.Get("datetime")
				assert.Equal(t, refTimeStr, val)

				// Check message (should contain original data)
				val, _ = d.Get("message")
				assert.Contains(t, val.(string), "bar")

				// Check timestamp_desc
				val, _ = d.Get("timestamp_desc")
				assert.Equal(t, "Velociraptor Event", val)

				// Check @timestamp and timestamp
				val, _ = d.Get("@timestamp")
				assert.Equal(t, refTimeNanoStr, val)

				val, _ = d.Get("timestamp")
				assert.Equal(t, refTimeMicro, val)
			},
		},
		{
			name: "Existing Fields Preserved if Valid",
			input: ordereddict.NewDict().
				Set("datetime", "2023-01-01T00:00:00Z").
				Set("message", "Original Message").
				Set("timestamp_desc", "Original Desc").
				Set("timestamp", refTimeStr),
			verify: func(t *testing.T, d *ordereddict.Dict) {
				val, _ := d.Get("datetime")
				assert.Equal(t, "2023-01-01T00:00:00Z", val)

				val, _ = d.Get("message")
				assert.Equal(t, "Original Message", val)

				val, _ = d.Get("timestamp_desc")
				assert.Equal(t, "Original Desc", val)

				// These are always overwritten based on determineTimestamp logic
				val, _ = d.Get("@timestamp")
				assert.Equal(t, refTimeNanoStr, val)
			},
		},
		{
			name: "Invalid Datetime Overwritten",
			input: ordereddict.NewDict().
				Set("datetime", "invalid-date").
				Set("timestamp", refTimeStr),
			verify: func(t *testing.T, d *ordereddict.Dict) {
				val, _ := d.Get("datetime")
				assert.Equal(t, refTimeStr, val)
			},
		},
		{
			name: "Non-String Message Converted",
			input: ordereddict.NewDict().
				Set("message", 12345).
				Set("timestamp", refTimeStr),
			verify: func(t *testing.T, d *ordereddict.Dict) {
				val, _ := d.Get("message")
				assert.Equal(t, "12345", val)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyTimesketchEnrichment(tt.input)
			tt.verify(t, tt.input)
		})
	}
}
