package utils

import "strings"

func ExtractHuntId(flow_id string) (string, bool) {
	parts := strings.Split(flow_id, ".")
	if len(parts) < 3 || parts[2] != "H" {
		return "", false
	}

	return "H." + parts[1], true
}

func CreateFlowIdFromHuntId(hunt_id string) string {
	parts := strings.SplitN(hunt_id, ".", 2)
	if len(parts) != 2 {
		return "F." + hunt_id
	}
	return "F." + parts[1] + ".H"

}
