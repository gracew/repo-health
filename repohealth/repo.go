package repohealth

import (
	"context"
	"log"
	"time"

	"github.com/google/go-github/github"
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

const numWeeks = 6
const pageSize = 100 // default is 30

func GetIssueScore(client *github.Client, owner string, repo string) []WeeklyIssueMetrics {
	since := time.Now().AddDate(0, 0, -7*numWeeks)
	issues := getIssuesSince(client, owner, repo, since)

	weekToNumIssuesOpened := map[int]int{}
	weekToNumIssuesClosed := map[int]int{}

	secondsInWeek := 60 * 60 * 24 * 7
	for _, issue := range issues {
		if issue.IsPullRequest() {
			continue
		}
		if issue.CreatedAt.After(since) {
			week := int(issue.CreatedAt.Sub(since).Seconds()) / secondsInWeek
			weekToNumIssuesOpened[week]++
		}
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

func getIssuesSince(client *github.Client, owner string, repo string, since time.Time) []*github.Issue {
	opts := &github.IssueListByRepoOptions{
		State:       "all",
		Since:       since,
		ListOptions: github.ListOptions{PerPage: pageSize},
	}
	issues, res, _ := client.Issues.ListByRepo(context.Background(), owner, repo, opts)
	log.Println(*res)
	for i := res.NextPage; i <= res.LastPage; i++ {
		opts.Page = i
		additionalIssues, res, _ := client.Issues.ListByRepo(context.Background(), owner, repo, opts)
		log.Println(*res)
		issues = append(issues, additionalIssues...)
	}
	return issues
}

func GetPRScore(client *github.Client, owner string, repo string) []WeeklyPRMetrics {
	since := time.Now().AddDate(0, 0, -7*numWeeks)
	prs := getPRsSince(client, owner, repo, since)

	weekToNumPRsOpened := map[int]int{}
	weekToNumPRsMerged := map[int]int{}
	weekToNumPRsRejected := map[int]int{}

	secondsInWeek := 60 * 60 * 24 * 7
	for _, pr := range prs {
		if pr.CreatedAt.After(since) {
			week := int(pr.CreatedAt.Sub(since).Seconds()) / secondsInWeek
			weekToNumPRsOpened[week]++
		}
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

func getPRsSince(client *github.Client, owner string, repo string, since time.Time) []*github.PullRequest {
	repoObj, _, _ := client.Repositories.Get(context.Background(), owner, repo)
	opts := &github.PullRequestListOptions{
		State:       "all",
		ListOptions: github.ListOptions{PerPage: pageSize},
		Base:        *repoObj.DefaultBranch,
		Sort:        "updated",
		Direction:   "desc",
	}
	prs, res, _ := client.PullRequests.List(context.Background(), owner, repo, opts)

	// unfortunately the list PRs endpoint doesn't support date filtering, so we do it ourselves. this returns a
	// few PRs older than the specified time, but we filter those out in the calling code...
	for i := res.NextPage; i <= res.LastPage; i++ {
		lastPr := prs[len(prs)-1]
		if lastPr.UpdatedAt.Before(since) {
			break
		}
		opts.Page = i
		additionalPRs, res, _ := client.PullRequests.List(context.Background(), owner, repo, opts)
		log.Println(*res)
		prs = append(prs, additionalPRs...)
	}

	return prs
}
