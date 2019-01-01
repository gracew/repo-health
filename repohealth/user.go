package repohealth

import "github.com/machinebox/graphql"

type UserScore struct {
	PRs []WeeklyPRMetrics `json:"prs"`
}

func GetUserPRScore(client *graphql.Client, authHeader string, user string, numWeeks int) []WeeklyPRMetrics {
	since := getStartDate(numWeeks)
	prs := getUserPRsCreatedSince(client, authHeader, user, since)
	return GetPRScore(prs, since, numWeeks)
}
