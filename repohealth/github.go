package repohealth

import (
	"context"
	"time"

	"github.com/machinebox/graphql"
	"github.com/pkg/errors"
)

type issueDatesResponse struct {
	Repository struct {
		Issues struct {
			Nodes    []issue
			PageInfo pageInfo
		}
	}
}

type issue struct {
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

func getIssuesCreatedSince(client *graphql.Client, authHeader string, owner string, name string, since time.Time) ([]issue, error) {
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
	req.Header.Set("Authorization", authHeader)

	var issues []issue
	getNextPage := true
	for getNextPage {
		var res issueDatesResponse
		if err := client.Run(context.Background(), req, &res); err != nil {
			return nil, errors.Wrap(err, "failed to fetch repo issues")
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

	return issues, nil
}

type defaultBranchResponse struct {
	Repository struct {
		DefaultBranchRef struct {
			Name string
		}
	}
}

type repoPRResponse struct {
	Repository struct {
		PullRequests struct {
			Nodes    []pr
			PageInfo pageInfo
		}
	}
}

type userPRResponse struct {
	User struct {
		PullRequests struct {
			Nodes    []pr
			PageInfo pageInfo
		}
	}
}

type pr struct {
	Number            int
	Title             string
	URL               string
	State             string
	CreatedAt         time.Time
	ClosedAt          time.Time
	Merged            bool
	IsCrossRepository bool
	Author            struct {
		Login string
	}
	Reviews struct {
		TotalCount int
		Nodes      []struct {
			CreatedAt time.Time
			Author    struct {
				Login string
			}
		}
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

const prFragment = `
	fragment prFields on PullRequestConnection {
		nodes {
			number
			title
			url
			state
			createdAt
			closedAt
			merged
			author @include(if: $byRepo) {
				login
			}
			reviews(first: 10) {
				totalCount
				nodes @include(if: $byRepo) {
					createdAt
					author {
						login
					}
				}
			}
		}
		pageInfo {
			endCursor
			hasNextPage
		}
	}
`

const prWithCIMetadataFragment = `
	fragment prFields on PullRequestConnection {
		nodes {
			number
			title
			url
			createdAt
			isCrossRepository
			commits(last: 1) @include(if: $byRepo) {
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
`

func getRepoPRsCreatedSince(client *graphql.Client, authHeader string, owner string, name string, since time.Time, prFragment string) ([]pr, error) {
	defaultBranchReq := graphql.NewRequest(`
		query ($owner: String!, $name: String!) {
			repository(owner: $owner, name: $name) {
				defaultBranchRef {
					name
				}
			}
		}
	`)
	defaultBranchReq.Var("owner", owner)
	defaultBranchReq.Var("name", name)
	defaultBranchReq.Header.Set("Authorization", authHeader)

	var defaultBranchRes defaultBranchResponse
	if err := client.Run(context.Background(), defaultBranchReq, &defaultBranchRes); err != nil {
		return nil, errors.Wrap(err, "failed to fetch default branch for repo")
	}

	req := graphql.NewRequest(`
		query ($owner: String!, $name: String!, $pageSize: Int!, $after: String, $defaultBranch: String!, $byRepo: Boolean = true) {
			repository(owner: $owner, name: $name) {
				pullRequests(first: $pageSize, after: $after, orderBy: {field: CREATED_AT, direction: DESC}, baseRefName: $defaultBranch) {
					...prFields
				}
			}
	  	}
	` + prFragment)
	req.Var("owner", owner)
	req.Var("name", name)
	req.Var("pageSize", pageSize)
	req.Var("after", nil)
	req.Var("defaultBranch", defaultBranchRes.Repository.DefaultBranchRef.Name)
	req.Header.Set("Authorization", authHeader)

	var prs []pr
	getNextPage := true
	for getNextPage {
		var res repoPRResponse
		if err := client.Run(context.Background(), req, &res); err != nil {
			return nil, errors.Wrap(err, "failed to fetch repo PRs")
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

	return prs, nil
}

func getUserPRsCreatedSince(client *graphql.Client, authHeader string, user string, since time.Time) ([]pr, error) {
	req := graphql.NewRequest(`
		query ($user: String!, $pageSize: Int!, $after: String, $byRepo: Boolean = false) {
			user(login: $user) {
				pullRequests(first: $pageSize, after: $after, orderBy: {field: CREATED_AT, direction: DESC}) {
					...prFields
				}
			}
	  	}
	` + prFragment)
	req.Var("user", user)
	req.Var("pageSize", pageSize)
	req.Var("after", nil)
	req.Header.Set("Authorization", authHeader)

	var prs []pr
	getNextPage := true
	for getNextPage {
		var res userPRResponse
		if err := client.Run(context.Background(), req, &res); err != nil {
			return nil, errors.Wrap(err, "failed to fetch user PRs")
		}
		newPrs := res.User.PullRequests.Nodes
		lastIndex := len(newPrs)
		for lastIndex > 0 && newPrs[lastIndex-1].CreatedAt.Before(since) {
			lastIndex--
		}
		prs = append(prs, newPrs[:lastIndex]...)
		getNextPage = lastIndex == len(newPrs) && res.User.PullRequests.PageInfo.HasNextPage
		req.Var("after", res.User.PullRequests.PageInfo.EndCursor)
	}

	return prs, nil
}
