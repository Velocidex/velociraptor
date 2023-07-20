package psutils

/*
  This is a wrapper package around gopsutils. gopsutils chooses to
  shell out to various commands (ps, lsof etc) on some platforms which
  makes it very hard to predict how expensive an operation is likely
  to be.

  The purpose of this wrapper is to better control what operations are
  allowed on different platforms to avoid this shelling behavior.

  We normally delegate through to gopsutils but in some situations we
  would prefer to fail the operations rather than shell out to
  external tools. Eventually we can remove all dependency on gopsutils
  from this wrapper and simply reimplement the API surface we need.
*/
