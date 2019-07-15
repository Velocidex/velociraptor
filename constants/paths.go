package constants

const (
	HUNTS_URN = "/hunts/"
)

func GetHuntURN(hunt_id string) string {
	return HUNTS_URN + hunt_id
}

func GetHuntStatsPath(hunt_id string) string {
	return HUNTS_URN + hunt_id + "/stats"
}
