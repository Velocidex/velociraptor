/*
  An implementation of the simple result selts. Simple results sets
  are written as JSON files with a row index file. There have the
  following properties:

  1. O(1) in access to a specific row - this allows fast paging of the
     table.
  2. Files are written in JSONL

*/

package simple
