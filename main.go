package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gracew/repo-health/repohealth"
	"github.com/julienschmidt/httprouter"
	"github.com/machinebox/graphql"
)

func main() {
	router := httprouter.New()
	router.GET("/login", login)
	router.GET("/repos/:org/:name", scoreRepository)
	if err := http.ListenAndServe(":8080", router); err != nil {
		panic(err)
	}
}

type githubTokenRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Code         string `json:"code"`
	State        string `json:"state"`
}

func login(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	queryValues := r.URL.Query()
	data, err := json.Marshal(githubTokenRequest{
		ClientID:     os.Getenv("CLIENT_ID"),
		ClientSecret: os.Getenv("CLIENT_SECRET"),
		Code:         queryValues.Get("code"),
		State:        queryValues.Get("state"),
	})
	if err != nil {
		log.Panicln(err)
	}

	req, err := http.NewRequest("POST", "https://github.com/login/oauth/access_token", bytes.NewBuffer(data))
	if err != nil {
		log.Panicln(err)
	}
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	res, err := client.Do(req)
	if err != nil {
		log.Panicln(err)
	}
	// TODO(gracew): remove once there's a proper dev setup
	w.Header().Set("Access-Control-Allow-Origin", "*")
	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Panicln(err)
	}
	w.Write(bytes)
}

func scoreRepository(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	client := graphql.NewClient("https://api.github.com/graphql")

	// TODO(gracew): if there's no auth header then return a 403...
	authHeader := r.Header["Authorization"][0]

	org := params.ByName("org")
	name := params.ByName("name")
	queryValues := r.URL.Query()
	numWeeks, err := strconv.Atoi(queryValues.Get("weeks"))
	if err != nil {
		log.Panicln(err)
	}

	issueScore := repohealth.GetIssueScore(client, authHeader, org, name, numWeeks)
	prScore, ciScore := repohealth.GetPRScore(client, authHeader, org, name, numWeeks)
	repoScore := repohealth.RepositoryScore{Issues: issueScore, PRs: prScore, CI: ciScore}
	// TODO(gracew): remove once there's a proper dev setup
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(repoScore)
}
