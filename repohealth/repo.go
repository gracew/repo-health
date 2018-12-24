package repohealth

import (
	"context"
	"log"
	"time"

	"github.com/machinebox/graphql"
	metrics "github.com/rcrowley/go-metrics"
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
	Week        string `json:"week"`
	BuildTime50 int64  `json:"buildTime50"` // in sec
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
	Commits   struct {
		Nodes []struct {
			Commit struct {
				PushedDate *time.Time
				Status     struct {
					Contexts []struct {
						Context   string
						CreatedAt *time.Time
					}
				}
			}
		}
	}
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
	getNextPage := true
	for getNextPage {
		var res issueDatesResponse
		if err := client.Run(context.Background(), req, &res); err != nil {
			log.Panic(err)
		}
		newIssues := res.Repository.Issues.Nodes
		lastIndex := len(newIssues)
		for lastIndex > 0 && newIssues[lastIndex-1].CreatedAt.Before(since) {
			lastIndex--
		}
		issues = append(issues, newIssues[:lastIndex]...)
		getNextPage = lastIndex == len(newIssues) && res.Repository.Issues.PageInfo.HasNextPage
		req.Var("after", res.Repository.Issues.PageInfo.EndCursor)
	}

	return issues
}

func GetPRScore(client *graphql.Client, owner string, name string, numWeeks int) ([]WeeklyPRMetrics, []WeeklyCIMetrics) {
	since := time.Now().AddDate(0, 0, -7*numWeeks)
	prs := getPRsCreatedSince(client, owner, name, since)

	weekToNumPRsOpened := map[int]int{}
	weekToNumPRsMerged := map[int]int{}
	weekToNumPRsRejected := map[int]int{}
	weekToCIHistogram := map[int]metrics.Histogram{}
	for week := 0; week < numWeeks; week++ {
		// just pick a really large reservoir size so we keep all measurements (one for each PR)
		weekToCIHistogram[week] = metrics.NewHistogram(metrics.NewUniformSample(10000))
	}

	secondsInWeek := 60 * 60 * 24 * 7
	for _, pr := range prs {
		createdWeek := int(pr.CreatedAt.Sub(since).Seconds()) / secondsInWeek
		weekToNumPRsOpened[createdWeek]++

		if pr.ClosedAt != nil && pr.ClosedAt.After(since) {
			closedWeek := int(pr.ClosedAt.Sub(since).Seconds()) / secondsInWeek
			if pr.MergedAt != nil {
				weekToNumPRsMerged[closedWeek]++
			} else {
				weekToNumPRsRejected[closedWeek]++
			}
		}

		maxCheckDuration := 0
		latestPRCommit := pr.Commits.Nodes[0].Commit
		for _, context := range latestPRCommit.Status.Contexts {
			duration := int(context.CreatedAt.Sub(*latestPRCommit.PushedDate).Seconds())
			if duration > maxCheckDuration {
				maxCheckDuration = duration
			}
		}
		pushedWeek := int(latestPRCommit.PushedDate.Sub(since).Seconds()) / secondsInWeek
		if pushedWeek < 0 {
			// commit may have been pushed before the PR was created; in this case use the PR creation date
			pushedWeek = createdWeek
		}
		weekToCIHistogram[pushedWeek].Update(int64(maxCheckDuration))
	}

	prMetrics := []WeeklyPRMetrics{}
	for week := 0; week < numWeeks; week++ {
		prMetrics = append(prMetrics, WeeklyPRMetrics{
			Week:        since.AddDate(0, 0, week*7).String(),
			NumOpen:     weekToNumPRsOpened[week],
			NumRejected: weekToNumPRsRejected[week],
			NumMerged:   weekToNumPRsMerged[week],
		})
	}

	ciMetrics := []WeeklyCIMetrics{}
	for week := 0; week < numWeeks; week++ {
		ciMetrics = append(ciMetrics, WeeklyCIMetrics{
			Week:        since.AddDate(0, 0, week*7).String(),
			BuildTime50: int64(weekToCIHistogram[week].Percentile(.5)),
		})
	}
	return prMetrics, ciMetrics
}

func getPRsCreatedSince(client *graphql.Client, owner string, name string, since time.Time) []prDates {
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
						commits(last: 1) {
							nodes {
								commit {
									pushedDate
									status {
										contexts {
											context
											createdAt
											state
										}
									}
								}
							}
						}
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
	getNextPage := true
	for getNextPage {
		var res prDatesResponse
		if err := client.Run(context.Background(), req, &res); err != nil {
			log.Panic(err)
		}
		newPrs := res.Repository.PullRequests.Nodes
		lastIndex := len(newPrs)
		for lastIndex > 0 && newPrs[lastIndex-1].CreatedAt.Before(since) {
			lastIndex--
		}
		prs = append(prs, newPrs[:lastIndex]...)
		getNextPage = lastIndex == len(newPrs) && res.Repository.PullRequests.PageInfo.HasNextPage
		req.Var("after", res.Repository.PullRequests.PageInfo.EndCursor)
	}

	return prs
}
