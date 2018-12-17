package repohealth

import (
	"context"
	"log"
	"time"

	"github.com/google/go-github/github"
)

type RepositoryScoreRequest struct {
	StartDate int64 `json:"startDate"` // in epoch sec
	EndDate   int64 `json:"endDate"`   // in epoch sec
}

type RepositoryScore struct {
	Issues []*WeeklyIssueMetrics `json:"issues"`
	PRs    []*WeeklyPRMetrics    `json:"prs"`
	CI     []*WeeklyCIMetrics    `json:"ci"`
}

type WeeklyIssueMetrics struct {
	Week            int64   `json:"week"` // in epoch sec
	NumClosed       int     `json:"closed"`
	NumOpen         int     `json:"open"`
	ResponseAfterMs []int64 `json:"responseAfterMs"`
	ResolvedAfterMs []int64 `json:"resolvedAfterMs"`
}

type WeeklyPRMetrics struct {
	Week            int64   `json:"week"` // in epoch sec
	NumMerged       int     `json:"merged"`
	NumClosed       int     `json:"close"`
	NumOpen         int     `json:"open"`
	NumReviews      int     `json:"numReviews"`
	ResponseAfterMs []int64 `json:"responseAfterMs"`
	ResolvedAfterMs []int64 `json:"resolvedAfter"`
}

type WeeklyCIMetrics struct {
	Week int64 `json:"week"` // in epoch sec
	// probably unreasonable to include all build times??
}

func GetIssueScore(client *github.Client, owner string, repo string) *WeeklyIssueMetrics {
	week := time.Now().AddDate(0, 0, -7)
	issues := getIssuesForWeek(client, owner, repo, week)
	numOpen := 0
	numClosed := 0

	for _, issue := range issues {
		if *issue.State == "open" {
			numOpen++
		} else {
			numClosed++
		}
	}

	return &WeeklyIssueMetrics{
		Week:      week.Unix(),
		NumOpen:   numOpen,
		NumClosed: numClosed,
	}
}

func getIssuesForWeek(client *github.Client, owner string, repo string, week time.Time) []*github.Issue {
	opts := &github.IssueListByRepoOptions{State: "all", Since: week}
	issues, res, _ := client.Issues.ListByRepo(context.Background(), owner, repo, opts)
	for i := res.NextPage; i <= res.LastPage; i++ {
		opts.Page = i
		addlIssues, res, _ := client.Issues.ListByRepo(context.Background(), owner, repo, opts)
		log.Println(*res)
		issues = append(issues, addlIssues...)
	}
	return issues
}
