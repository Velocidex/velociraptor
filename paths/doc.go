// This package contains functions that map objects into the filestore
// namespace.

/*
  Since Velociraptor uses a simple file system to manage all its data,
  we need a way to define which type of data is stored in which
  file. This directory contains "path managers" - simple builder
  pattern objects which calculate the location of various files.

  In a sense, the path managers define the data schema - essentially
  how data will be stored on the filesystem.

  Here exist a path manager for each type of object. The manager has
  methods to return the storage paths of various files related to that
  object and their types.

  For example the ClientPathManager manages the location of the
  client's keys, last ping time record, client info record, etc. Each
  of these records has a different filesystem path.
*/

package paths
