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
// Velociraptor's flavour of Comma Separated Value (CSV) files.

// Velociraptor requires a file format that stores tabulated data. CSV
// seems like the perfect choice because it is well supported by
// virtually every tool (spreadsheets, databases etc).

// Pros:

//  1. CSV are just flat files, which can be appended to without
//     modifying previous data. This is perfect for many uses within
//     Velociraptor: For example as more data is retreived from
//     clients, we can just append to the end of an existing CSV file.

//  2. CSV Readers can watch the end of the file and when it grows
//     they can read the last few rows from it. As long as we can
//     guarantee that a line must be terminated by a line feed, then
//     we can detect partially written records (without a linefeed at
//     the end of the file). Therefore we can follow the CSV file
//     without requiring any file locks.

// Cons:

//  1. Unfortunately CSV does not preserve types (e.g. like JSON). A
//     number (e.g. 12) may be a string with that value or an int. The
//     reader has no idea. It is also hard to store more complex types
//     (byte array, maps, lists etc). CSV can only store strings.

// Therefore Velociraptor maintains its own flavor of CSV which
// specifies more accurately how it should be encoded. The goal is to
// be able to reliably round trip typed data through a CSV file, while
// at the same time, produce a CSV file which is mostly useful to
// other programs without requiring processing.

// Encoding rules:
//
// We convert arbitrary types to string value according to the
// following rules:

// 1. Integers and floats are converted to strings (e.g. 12 -> "12")

// 2. If a string looks like a number (e.g. "12", or starts with "{"
//    or "[" or " ") we prepend it with a space. These characters will
//    be treated specially when decoding so we do not want spurious
//    strings being confused with these. At the same time programs
//    which parse CSV generally ignore leading space (although it is
//    preserved in the CSV).

// 3. Byte arrays are base64 encoded and have "base64:" prepended.

// 4. Other objects are encoded into json.

// When decoding we reverse the process:
// 1. If the field starts with "base64:" we convert it to byte array.
// 2. If the field starts with a space, we consume the space.
// 3. If the field starts with [ or { we decode the json string.
// 4. If the field looks like an integer or float we convert it.

// This package also modifies the CSV reader to be able to retry the
// last record if it is not valid. This is important for tailing a
// growing CSV file.

// Imagine the CSV writer buffers many records, then flushes its
// buffers. It is possible (likely in fact) that the last record is
// not complete:

// a,b,c\n
// x,y

// By enabling the RequireLineSeperator flag on the reader we can
// ensure that the last line is rejected since it has no line
// separator (and our writer always writes one).

// Additionally each successfully read line will update the reader's
// ByteOffset member to point at the start of the next row to read. If
// a read failed (due to the record being incomplete), the ByteOffset
// member is not updated. It is therefore possible to Seek() the csv
// reader back to the position of the bad row and attempt to read it
// again - hopefully the writer would flush its next buffer by then.

package csv
