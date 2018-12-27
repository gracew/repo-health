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
	Week      string         `json:"week"`
	NumClosed int            `json:"closed"`
	NumOpen   int            `json:"opened"`
	Details   []IssueDetails `json:"details"`
}

type IssueDetails struct {
	Issue          int `json:"issue"`
	ResolutionTime int `json:"resolutionTime"` // in sec
}

type WeeklyPRMetrics struct {
	Week        string      `json:"week"`
	NumMerged   int         `json:"merged"`
	NumRejected int         `json:"rejected"`
	NumOpen     int         `json:"opened"`
	Details     []PRDetails `json:"details"`
}

type PRDetails struct {
	PR             int `json:"pr"`
	ResolutionTime int `json:"resolutionTime"` // in sec
	NumReviews     int `json:"reviews"`
}

type WeeklyCIMetrics struct {
	Week    string      `json:"week"`
	Details []CIDetails `json:"details"`
}

type CIDetails struct {
	PR               int    `json:"pr"`
	MaxCheckName     string `json:"maxCheckName"`
	MaxCheckDuration int    `json:"maxCheckDuration"` // in sec
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
	CreatedAt time.Time
	ClosedAt  time.Time
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
	Number            int
	CreatedAt         time.Time
	ClosedAt          time.Time
	Merged            bool
	IsCrossRepository bool
	Reviews           struct {
		TotalCount int
	}
	Commits struct {
		Nodes []struct {
			Commit struct {
				CommittedDate time.Time
				PushedDate    time.Time
				Status        struct {
					Contexts []struct {
						Context   string
						CreatedAt time.Time
					}
				}
			}
		}
	}
}

const pageSize = 100 // default is 30
const dateFormat = "2006-01-02"

func GetIssueScore(client *graphql.Client, owner string, name string, numWeeks int) []WeeklyIssueMetrics {
	since := time.Now().AddDate(0, 0, -7*numWeeks)
	issues := getIssuesCreatedSince(client, owner, name, since)

	weekToNumIssuesOpened := map[int]int{}
	weekToNumIssuesClosed := map[int]int{}
	weekToIssueDetails := map[int][]IssueDetails{}

	secondsInWeek := 60 * 60 * 24 * 7
	for _, issue := range issues {
		createdWeek := int(issue.CreatedAt.Sub(since).Seconds()) / secondsInWeek
		weekToNumIssuesOpened[createdWeek]++

		resolutionTime := -1
		if issue.ClosedAt.After(since) {
			closedWeek := int(issue.ClosedAt.Sub(since).Seconds()) / secondsInWeek
			weekToNumIssuesClosed[closedWeek]++
			resolutionTime = int(issue.ClosedAt.Sub(issue.CreatedAt).Seconds())
		}

		weekToIssueDetails[createdWeek] = append(weekToIssueDetails[createdWeek], IssueDetails{
			Issue:          issue.Number,
			ResolutionTime: resolutionTime,
		})
	}

	metrics := []WeeklyIssueMetrics{}
	for week := 0; week < numWeeks; week++ {
		metrics = append(metrics, WeeklyIssueMetrics{
			Week:      since.AddDate(0, 0, week*7).Format(dateFormat),
			NumOpen:   weekToNumIssuesOpened[week],
			NumClosed: weekToNumIssuesClosed[week],
			Details:   weekToIssueDetails[week],
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
	weekToPRDetails := map[int][]PRDetails{}
	weekToCIDetails := map[int][]CIDetails{}

	secondsInWeek := 60 * 60 * 24 * 7
	for _, pr := range prs {
		createdWeek := int(pr.CreatedAt.Sub(since).Seconds()) / secondsInWeek
		weekToNumPRsOpened[createdWeek]++

		resolutionTime := -1
		if pr.ClosedAt.After(since) {
			closedWeek := int(pr.ClosedAt.Sub(since).Seconds()) / secondsInWeek
			if pr.Merged {
				weekToNumPRsMerged[closedWeek]++
			} else {
				weekToNumPRsRejected[closedWeek]++
			}
			resolutionTime = int(pr.ClosedAt.Sub(pr.CreatedAt).Seconds())
		}
		weekToPRDetails[createdWeek] = append(weekToPRDetails[createdWeek], PRDetails{
			PR:             pr.Number,
			ResolutionTime: resolutionTime,
			NumReviews:     pr.Reviews.TotalCount,
		})

		latestPRCommit := pr.Commits.Nodes[0].Commit
		var statusStartDate time.Time
		if pr.IsCrossRepository {
			// pushed date is unavailable for PRs made from forks; use the commit date instead
			if pr.CreatedAt.After(latestPRCommit.CommittedDate) {
				// first commit; build isn't triggered until PR creation so use that date
				statusStartDate = pr.CreatedAt
			} else {
				// not ideal in case commit is made awhile before pushing. however, updatedAt also includes comments,
				// reviews, etc. can possibly traverse PR timeline to get a more accurate date
				statusStartDate = latestPRCommit.CommittedDate
			}
		} else {
			statusStartDate = latestPRCommit.PushedDate
		}

		maxCheckDuration := 0
		var maxCheckContext string
		for _, context := range latestPRCommit.Status.Contexts {
			duration := int(context.CreatedAt.Sub(statusStartDate).Seconds())
			if duration > maxCheckDuration {
				maxCheckDuration = duration
				maxCheckContext = context.Context
			}
		}
		statusStartWeek := int(statusStartDate.Sub(since).Seconds()) / secondsInWeek
		if statusStartWeek < 0 {
			// commit may have been pushed before the PR was created; in this case use the PR creation date
			statusStartWeek = createdWeek
		}
		weekToCIDetails[statusStartWeek] = append(weekToCIDetails[statusStartWeek], CIDetails{
			PR:               pr.Number,
			MaxCheckName:     maxCheckContext,
			MaxCheckDuration: maxCheckDuration,
		})
	}

	prMetrics := []WeeklyPRMetrics{}
	ciMetrics := []WeeklyCIMetrics{}
	for week := 0; week < numWeeks; week++ {
		prMetrics = append(prMetrics, WeeklyPRMetrics{
			Week:        since.AddDate(0, 0, week*7).Format(dateFormat),
			NumOpen:     weekToNumPRsOpened[week],
			NumRejected: weekToNumPRsRejected[week],
			NumMerged:   weekToNumPRsMerged[week],
			Details:     weekToPRDetails[week],
		})

		ciMetrics = append(ciMetrics, WeeklyCIMetrics{
			Week:    since.AddDate(0, 0, week*7).Format(dateFormat),
			Details: weekToCIDetails[week],
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
						merged
						isCrossRepository
						reviews(first: 1) {
							totalCount
						}
						commits(last: 1) {
							nodes {
								commit {
									committedDate
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
