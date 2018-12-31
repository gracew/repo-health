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
	Number         int    `json:"number"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	ResolutionTime int    `json:"resolutionTime"` // in sec, will be -1 if issue has not yet been resolved
	State          string `json:"state"`
}

type WeeklyPRMetrics struct {
	Week        string      `json:"week"`
	NumMerged   int         `json:"merged"`
	NumRejected int         `json:"rejected"`
	NumOpen     int         `json:"opened"`
	Details     []PRDetails `json:"details"`
}

type PRDetails struct {
	Number         int    `json:"number"`
	Title          string `json:"title"`
	URL            string `json:"url"`
	ResolutionTime int    `json:"resolutionTime"` // in sec, will be -1 if PR has not yet been resolved
	NumReviews     int    `json:"reviews"`
	State          string `json:"state"`
}

type WeeklyCIMetrics struct {
	Week    string      `json:"week"`
	Details []CIDetails `json:"details"`
}

type CIDetails struct {
	PR               int    `json:"pr"`
	PRURL            string `json:"prUrl"`
	MaxCheckName     string `json:"maxCheckName"`
	MaxCheckDuration int    `json:"maxCheckDuration"` // in sec
	MaxCheckURL      string `json:"maxCheckUrl"`
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
	Title     string
	URL       string
	State     string
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
	Title             string
	URL               string
	State             string
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
					Contexts []checkContext
				}
			}
		}
	}
}

type checkContext struct {
	Context   string
	CreatedAt time.Time
	TargetURL string
}

const pageSize = 100 // default is 30
const dateFormat = "2006-01-02"

// returns the first Sunday after (today - numWeeks)
func getStartDate(numWeeks int) time.Time {
	since := time.Now().AddDate(0, 0, -7*numWeeks)
	for since.Weekday() != 0 {
		since = since.AddDate(0, 0, 1)
	}
	return since
}

func GetIssueScore(client *graphql.Client, authHeader string, owner string, name string, numWeeks int) []WeeklyIssueMetrics {
	since := getStartDate(numWeeks)
	issues := getIssuesCreatedSince(client, authHeader, owner, name, since)

	weekToNumIssuesOpened := map[int]int{}
	weekToNumIssuesClosed := map[int]int{}
	weekToIssueDetails := map[int][]IssueDetails{}

	secondsInWeek := 60 * 60 * 24 * 7
	for _, issue := range issues {
		createdWeek := int(issue.CreatedAt.Sub(since).Seconds()) / secondsInWeek
		weekToNumIssuesOpened[createdWeek]++

		if issue.ClosedAt.After(since) {
			closedWeek := int(issue.ClosedAt.Sub(since).Seconds()) / secondsInWeek
			weekToNumIssuesClosed[closedWeek]++
			weekToIssueDetails[createdWeek] = append(weekToIssueDetails[createdWeek], IssueDetails{
				Number:         issue.Number,
				Title:          issue.Title,
				URL:            issue.URL,
				ResolutionTime: int(issue.ClosedAt.Sub(issue.CreatedAt).Seconds()),
			})
		}

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

func getIssuesCreatedSince(client *graphql.Client, authHeader string, owner string, name string, since time.Time) []issueDates {
	req := graphql.NewRequest(`
		query ($owner: String!, $name: String!, $pageSize: Int!, $after: String) {
			repository(owner: $owner, name: $name) {
		 		issues(first: $pageSize, after: $after, orderBy: {field: CREATED_AT, direction: DESC}) {
					nodes {
						number
						title
						url
						state
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
	req.Header["Authorization"] = append(req.Header["Authorization"], authHeader)

	var issues []issueDates
	getNextPage := true
	for getNextPage {
		var res issueDatesResponse
		if err := client.Run(context.Background(), req, &res); err != nil {
			log.Panicln(err)
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

func GetPRScore(client *graphql.Client, authHeader string, owner string, name string, numWeeks int) ([]WeeklyPRMetrics, []WeeklyCIMetrics) {
	since := getStartDate(numWeeks)
	prs := getPRsCreatedSince(client, authHeader, owner, name, since)

	weekToNumPRsOpened := map[int]int{}
	weekToNumPRsMerged := map[int]int{}
	weekToNumPRsRejected := map[int]int{}
	weekToPRDetails := map[int][]PRDetails{}
	weekToCIDetails := map[int][]CIDetails{}

	secondsInWeek := 60 * 60 * 24 * 7
	for _, pr := range prs {
		createdWeek := int(pr.CreatedAt.Sub(since).Seconds()) / secondsInWeek
		weekToNumPRsOpened[createdWeek]++

		if pr.ClosedAt.After(since) {
			closedWeek := int(pr.ClosedAt.Sub(since).Seconds()) / secondsInWeek
			if pr.Merged {
				weekToNumPRsMerged[closedWeek]++
			} else {
				weekToNumPRsRejected[closedWeek]++
			}
			weekToPRDetails[createdWeek] = append(weekToPRDetails[createdWeek], PRDetails{
				Number:         pr.Number,
				Title:          pr.Title,
				URL:            pr.URL,
				State:          pr.State,
				ResolutionTime: int(pr.ClosedAt.Sub(pr.CreatedAt).Seconds()),
				NumReviews:     pr.Reviews.TotalCount,
			})
		}

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
		var maxCheckContext checkContext
		for _, context := range latestPRCommit.Status.Contexts {
			duration := int(context.CreatedAt.Sub(statusStartDate).Seconds())
			if duration > maxCheckDuration {
				maxCheckDuration = duration
				maxCheckContext = context
			}
		}
		statusStartWeek := int(statusStartDate.Sub(since).Seconds()) / secondsInWeek
		if statusStartWeek < 0 {
			// commit may have been pushed before the PR was created; in this case use the PR creation date
			statusStartWeek = createdWeek
		}
		weekToCIDetails[statusStartWeek] = append(weekToCIDetails[statusStartWeek], CIDetails{
			PR:               pr.Number,
			PRURL:            pr.URL,
			MaxCheckName:     maxCheckContext.Context,
			MaxCheckDuration: maxCheckDuration,
			MaxCheckURL:      maxCheckContext.TargetURL,
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

func getPRsCreatedSince(client *graphql.Client, authHeader string, owner string, name string, since time.Time) []prDates {
	// TODO(gracew): look up the repo's default branch and use that in the query
	req := graphql.NewRequest(`
		query ($owner: String!, $name: String!, $pageSize: Int!, $after: String) {
			repository(owner: $owner, name: $name) {
		 		pullRequests(first: $pageSize, after: $after, orderBy: {field: CREATED_AT, direction: DESC}, baseRefName: "master") {
					nodes {
						number
						title
						url
						state
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
											targetUrl
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
	req.Header["Authorization"] = append(req.Header["Authorization"], authHeader)

	var prs []prDates
	getNextPage := true
	for getNextPage {
		var res prDatesResponse
		if err := client.Run(context.Background(), req, &res); err != nil {
			log.Panicln(err)
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
