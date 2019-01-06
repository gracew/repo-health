package repohealth

import (
	"time"
)

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
	ReviewTime     int    `json:"reviewTime"`     // in sec, will be -1 if PR has not yet been reviewed
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

const pageSize = 100 // default is 30
const dateFormat = "2006-01-02"

func GetIssueScore(issues []issue, since time.Time, numWeeks int) []WeeklyIssueMetrics {
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

func GetPRScore(prs []pr, since time.Time, numWeeks int) []WeeklyPRMetrics {
	weekToNumPRsOpened := map[int]int{}
	weekToNumPRsMerged := map[int]int{}
	weekToNumPRsRejected := map[int]int{}
	weekToPRDetails := map[int][]PRDetails{}
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

		reviewTime := -1
		for _, review := range pr.Reviews.Nodes {
			if review.Author.Login != pr.Author.Login {
				reviewTime = int(review.CreatedAt.Sub(pr.CreatedAt).Seconds())
				break
			}
		}

		weekToPRDetails[createdWeek] = append(weekToPRDetails[createdWeek], PRDetails{
			Number:         pr.Number,
			Title:          pr.Title,
			URL:            pr.URL,
			State:          pr.State,
			ResolutionTime: resolutionTime,
			ReviewTime:     reviewTime,
			NumReviews:     pr.Reviews.TotalCount,
		})
	}

	prMetrics := []WeeklyPRMetrics{}
	for week := 0; week < numWeeks; week++ {
		prMetrics = append(prMetrics, WeeklyPRMetrics{
			Week:        since.AddDate(0, 0, week*7).Format(dateFormat),
			NumOpen:     weekToNumPRsOpened[week],
			NumRejected: weekToNumPRsRejected[week],
			NumMerged:   weekToNumPRsMerged[week],
			Details:     weekToPRDetails[week],
		})
	}

	return prMetrics
}

func GetCIScore(prs []pr, since time.Time, numWeeks int) []WeeklyCIMetrics {
	weekToCIDetails := map[int][]CIDetails{}

	secondsInWeek := 60 * 60 * 24 * 7
	for _, pr := range prs {
		createdWeek := int(pr.CreatedAt.Sub(since).Seconds()) / secondsInWeek
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

	ciMetrics := []WeeklyCIMetrics{}
	for week := 0; week < numWeeks; week++ {
		ciMetrics = append(ciMetrics, WeeklyCIMetrics{
			Week:    since.AddDate(0, 0, week*7).Format(dateFormat),
			Details: weekToCIDetails[week],
		})
	}

	return ciMetrics
}
