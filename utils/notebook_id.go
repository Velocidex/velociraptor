package utils

import (
	"regexp"
	"strings"
)

// A notebook id for clients flows
var (
	client_notebook_regex = regexp.MustCompile(`^N\.(F\.[^-]+?)-(.+|server)$`)
	event_notebook_regex  = regexp.MustCompile(`^N\.E\.([^-]+?)-(.+|server)$`)
)

func ClientNotebookId(notebook_id string) (flow_id, client_id string, ok bool) {
	matches := client_notebook_regex.FindStringSubmatch(notebook_id)
	if len(matches) == 3 {
		return matches[1], matches[2], true
	}

	return "", "", false
}

func EventNotebookId(notebook_id string) (artifact, client_id string, ok bool) {
	matches := event_notebook_regex.FindStringSubmatch(notebook_id)
	if len(matches) == 3 {
		return matches[1], matches[2], true
	}

	return "", "", false
}

func HuntNotebookId(notebook_id string) (hunt_id string, ok bool) {
	if strings.HasPrefix(notebook_id, "N.H.") {
		return strings.TrimPrefix(notebook_id, "N."), true
	}
	return "", false
}

func DashboardNotebookId(notebook_id string) (ok bool) {
	return strings.HasPrefix(notebook_id, "Dashboard")
}

func IsGlobalNotebooks(notebook_id string) bool {
	if strings.HasPrefix(notebook_id, "N.F.") || // Flow Notebook
		strings.HasPrefix(notebook_id, "N.H.") || // Hunt Notebook
		strings.HasPrefix(notebook_id, "N.E.") { // Event Notebook
		return false
	}

	return true
}
