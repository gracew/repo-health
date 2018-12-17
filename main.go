package main

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/gracew/repo-health/repohealth"
	"github.com/julienschmidt/httprouter"

	"github.com/google/go-github/github"
	"golang.org/x/oauth2"
)

func main() {
	router := httprouter.New()
	router.GET("/:org/:repo/score", scoreRepository)
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
	client := github.NewClient(oauthClient)

	issueScore := repohealth.GetIssueScore(client, params.ByName("org"), params.ByName("repo"))
	json.NewEncoder(w).Encode(issueScore)
}
