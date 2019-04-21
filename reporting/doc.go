package reporting

// This package implements reporting and post processing of collected
// artifacts.

// The reporting is defined within the artifact using a
// template. Velociraptor will format the report on the server using
// the template system implemented in this module.

// # Report types

// There are a number of different situations that require reporting
// and post processing. For example, when running the artifact by
// itself we might have a different focus than when running a hunt.

// ## MONITORING_DAILY reports

// These reports are intended to operate on a single daily monitoring
// log. Since the log file is typically small these reports should be
// fast to run.

// # The templating system

// Velociraptor uses Go's templating facility to provide the user with
// a fully functioning templating system. The template is able to
// execute VQL statements and therefore can do anything.

// NOTE: Being able to define reports within artifacts provides the
// user with full control (shell access!) of the server. This happens
// because VQL in reports runs server side and can access the entire
// VQL functionality.

// Velociraptor currently does not have finer grain access control -
// read only users are unable to modify any artifacts and full users
// can run anything on the server.
