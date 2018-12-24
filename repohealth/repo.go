package repohealth

import (
	"context"
	"log"
	"time"

	"github.com/machinebox/graphql"
)

type RepositoryScore struct {
	Issues []WeeklyIssueMetrics `json:"issues"`
	PRs    []WeeklyPRMetrics    `json:"prs"`
	CI     []WeeklyCIMetrics    `json:"ci"`
}

type WeeklyIssueMetrics struct {
	Week             string  `json:"week"`
	NumClosed        int     `json:"closed"`
	NumOpen          int     `json:"opened"`
	TimeToResponse   []int64 `json:"timeToResponse"`   // in sec
	TimeToResolution []int64 `json:"timeToResolution"` // in sec
}

type WeeklyPRMetrics struct {
	Week             string  `json:"week"`
	NumMerged        int     `json:"merged"`
	NumRejected      int     `json:"rejected"`
	NumOpen          int     `json:"opened"`
	NumReviews       int     `json:"numReviews"`
	TimeToResponse   []int64 `json:"timeToResponse"`   // in sec
	TimeToResolution []int64 `json:"timeToResolution"` // in sec
}

type WeeklyCIMetrics struct {
	Week string `json:"week"`
	// probably unreasonable to include all build times??
}

type issueDatesResponse struct {
	Repository struct {
		Issues struct {
			Nodes    []issueDates
			PageInfo pageInfo
		}
	}
}

type issueDates struct {
	Number    int
	CreatedAt *time.Time
	ClosedAt  *time.Time
}

type pageInfo struct {
	EndCursor   string
	HasNextPage bool
}

type prDatesResponse struct {
	Repository struct {
		PullRequests struct {
			Nodes    []prDates
			PageInfo pageInfo
		}
	}
}

type prDates struct {
	Number    int
	CreatedAt *time.Time
	ClosedAt  *time.Time
	MergedAt  *time.Time
}

const pageSize = 100 // default is 30

func GetIssueScore(client *graphql.Client, owner string, name string, numWeeks int) []WeeklyIssueMetrics {
	since := time.Now().AddDate(0, 0, -7*numWeeks)
	issues := getIssuesCreatedSince(client, owner, name, since)

	weekToNumIssuesOpened := map[int]int{}
	weekToNumIssuesClosed := map[int]int{}

	secondsInWeek := 60 * 60 * 24 * 7
	for _, issue := range issues {
		week := int(issue.CreatedAt.Sub(since).Seconds()) / secondsInWeek
		weekToNumIssuesOpened[week]++

		if issue.ClosedAt != nil && issue.ClosedAt.After(since) {
			week := int(issue.ClosedAt.Sub(since).Seconds()) / secondsInWeek
			weekToNumIssuesClosed[week]++
		}
	}

	metrics := []WeeklyIssueMetrics{}
	for week := 0; week < numWeeks; week++ {
		metrics = append(metrics, WeeklyIssueMetrics{
			Week:      since.AddDate(0, 0, week*7).String(),
			NumOpen:   weekToNumIssuesOpened[week],
			NumClosed: weekToNumIssuesClosed[week],
		})
	}
	return metrics
}

// this returns a few issues older than the specified time, but we filter those out in the calling code...
func getIssuesCreatedSince(client *graphql.Client, owner string, name string, since time.Time) []issueDates {
	req := graphql.NewRequest(`
		query ($owner: String!, $name: String!, $pageSize: Int!, $after: String) {
			repository(owner: $owner, name: $name) {
		 		issues(first: $pageSize, after: $after, orderBy: {field: CREATED_AT, direction: DESC}) {
					nodes {
						number
						createdAt
						closedAt
					}
					pageInfo {
						endCursor
						hasNextPage
					}
				}
			}
	  	}
	`)
	req.Var("owner", owner)
	req.Var("name", name)
	req.Var("pageSize", pageSize)
	req.Var("after", nil)

	var issues []issueDates
	hasNextPage := true
	for hasNextPage {
		if len(issues) > 0 && issues[len(issues)-1].CreatedAt.Before(since) {
			break
		}
		var res issueDatesResponse
		if err := client.Run(context.Background(), req, &res); err != nil {
			log.Panic(err)
		}
		issues = append(issues, res.Repository.Issues.Nodes...)
		hasNextPage = res.Repository.Issues.PageInfo.HasNextPage
		req.Var("after", res.Repository.Issues.PageInfo.EndCursor)
	}

	return issues
}

func GetPRScore(client *graphql.Client, owner string, name string, numWeeks int) []WeeklyPRMetrics {
	since := time.Now().AddDate(0, 0, -7*numWeeks)
	prs := getPRsSince(client, owner, name, since)

	weekToNumPRsOpened := map[int]int{}
	weekToNumPRsMerged := map[int]int{}
	weekToNumPRsRejected := map[int]int{}

	secondsInWeek := 60 * 60 * 24 * 7
	for _, pr := range prs {
		week := int(pr.CreatedAt.Sub(since).Seconds()) / secondsInWeek
		weekToNumPRsOpened[week]++

		if pr.ClosedAt != nil && pr.ClosedAt.After(since) {
			week := int(pr.ClosedAt.Sub(since).Seconds()) / secondsInWeek
			if pr.MergedAt != nil {
				weekToNumPRsMerged[week]++
			} else {
				weekToNumPRsRejected[week]++
			}
		}
	}

	metrics := []WeeklyPRMetrics{}
	for week := 0; week < numWeeks; week++ {
		metrics = append(metrics, WeeklyPRMetrics{
			Week:        since.AddDate(0, 0, week*7).String(),
			NumOpen:     weekToNumPRsOpened[week],
			NumRejected: weekToNumPRsRejected[week],
			NumMerged:   weekToNumPRsMerged[week],
		})
	}
	return metrics
}

// this returns a few PRs older than the specified time, but we filter those out in the calling code...
func getPRsSince(client *graphql.Client, owner string, name string, since time.Time) []prDates {
	// TODO(gracew): look up the repo's default branch and use that in the query
	req := graphql.NewRequest(`
		query ($owner: String!, $name: String!, $pageSize: Int!, $after: String) {
			repository(owner: $owner, name: $name) {
		 		pullRequests(first: $pageSize, after: $after, orderBy: {field: CREATED_AT, direction: DESC}, baseRefName: "master") {
					nodes {
						number
						createdAt
						closedAt
						mergedAt
					}
					pageInfo {
						endCursor
						hasNextPage
					}
				}
			}
	  	}
	`)
	req.Var("owner", owner)
	req.Var("name", name)
	req.Var("pageSize", pageSize)
	req.Var("after", nil)

	var prs []prDates
	hasNextPage := true
	for hasNextPage {
		if len(prs) > 0 && prs[len(prs)-1].CreatedAt.Before(since) {
			break
		}
		var res prDatesResponse
		if err := client.Run(context.Background(), req, &res); err != nil {
			log.Panic(err)
		}
		prs = append(prs, res.Repository.PullRequests.Nodes...)
		hasNextPage = res.Repository.PullRequests.PageInfo.HasNextPage
		req.Var("after", res.Repository.PullRequests.PageInfo.EndCursor)
	}

	return prs
}
