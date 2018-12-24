package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gracew/repo-health/repohealth"
	"github.com/julienschmidt/httprouter"
	"github.com/machinebox/graphql"

	"golang.org/x/oauth2"
)

func main() {
	router := httprouter.New()
	router.GET("/:org/:name/score", scoreRepository)
	if err := http.ListenAndServe(":8080", router); err != nil {
		panic(err)
	}
}

// copied from https://blog.kowalczyk.info/article/f/accessing-github-api-from-go.html
type TokenSource struct {
	AccessToken string
}

func (t *TokenSource) Token() (*oauth2.Token, error) {
	token := &oauth2.Token{
		AccessToken: t.AccessToken,
	}
	return token, nil
}

func scoreRepository(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	tokenSource := &TokenSource{
		AccessToken: os.Getenv("ACCESS_TOKEN"),
	}
	oauthClient := oauth2.NewClient(oauth2.NoContext, tokenSource)
	client := graphql.NewClient("https://api.github.com/graphql", graphql.WithHTTPClient(oauthClient))

	org := params.ByName("org")
	name := params.ByName("name")
	queryValues := r.URL.Query()
	numWeeks, err := strconv.Atoi(queryValues.Get("weeks"))
	if err != nil {
		log.Panic(err)
	}
	issueScore := repohealth.GetIssueScore(client, org, name, numWeeks)
	prScore := repohealth.GetPRScore(client, org, name, numWeeks)
	repoScore := repohealth.RepositoryScore{Issues: issueScore, PRs: prScore}
	json.NewEncoder(w).Encode(repoScore)
}
