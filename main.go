package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gracew/repo-health/repohealth"
	"github.com/julienschmidt/httprouter"
)

func main() {
	router := httprouter.New()
	router.GET("/login", login)

	router.GET("/repos/:owner/:name/issues", requireAuthHeader(repohealth.GetRepositoryIssues))

	router.GET("/repos/:owner/:name/prs", requireAuthHeader(repohealth.GetRepositoryPRs))

	router.GET("/repos/:owner/:name/ci", requireAuthHeader(repohealth.GetRepositoryCI))

	router.GET("/users/:user", requireAuthHeader(repohealth.GetUserPRs))

	if err := http.ListenAndServe(":8080", router); err != nil {
		panic(err)
	}
}

func requireAuthHeader(handler httprouter.Handle) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		handler(w, r, ps)
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
	req.Header.Set("Content-type", "application/json")

	client := &http.Client{Timeout: time.Minute}
	res, err := client.Do(req)
	if err != nil {
		log.Panicln(err)
	}
	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Panicln(err)
	}
	w.Write(bytes)
}
