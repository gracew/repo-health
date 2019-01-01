package repohealth

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/machinebox/graphql"
)

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

const ciFragment = `
	fragment ciFields on PullRequest {
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
`

func getPRFragment(includeCIFields bool) string {
	fragment := `
		fragment prFields on PullRequestConnection {
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
				%s
			}
			pageInfo {
				endCursor
				hasNextPage
			}
		}
		%s
	`
	if includeCIFields {
		return fmt.Sprintf(fragment, "...ciFields", ciFragment)
	}
	return fmt.Sprintf(fragment, "", "")
}

func getRepoPRsCreatedSince(client *graphql.Client, authHeader string, owner string, name string, since time.Time) []pr {
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
		log.Panicln(err)
	}

	req := graphql.NewRequest(`
		query ($owner: String!, $name: String!, $pageSize: Int!, $after: String, $defaultBranch: String!) {
			repository(owner: $owner, name: $name) {
				pullRequests(first: $pageSize, after: $after, orderBy: {field: CREATED_AT, direction: DESC}, baseRefName: $defaultBranch) {
					...prFields
				}
			}
	  	}
	` + getPRFragment(true))
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

func getUserPRsCreatedSince(client *graphql.Client, authHeader string, user string, since time.Time) []pr {
	req := graphql.NewRequest(`
		query ($user: String!, $pageSize: Int!, $after: String) {
			user(login: $user) {
				pullRequests(first: $pageSize, after: $after, orderBy: {field: CREATED_AT, direction: DESC}) {
					...prFields
				}
			}
	  	}
	` + getPRFragment(false))
	req.Var("user", user)
	req.Var("pageSize", pageSize)
	req.Var("after", nil)
	req.Header.Set("Authorization", authHeader)

	var prs []pr
	getNextPage := true
	for getNextPage {
		var res userPRResponse
		if err := client.Run(context.Background(), req, &res); err != nil {
			log.Panicln(err)
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

	return prs
}
