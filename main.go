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

	router.OPTIONS("/repos/:owner/:name/issues", allowCors)
	router.GET("/repos/:owner/:name/issues", repohealth.GetRepositoryIssues)

	router.OPTIONS("/repos/:owner/:name/prs", allowCors)
	router.GET("/repos/:owner/:name/prs", repohealth.GetRepositoryPRs)

	router.OPTIONS("/repos/:owner/:name/ci", allowCors)
	router.GET("/repos/:owner/:name/ci", repohealth.GetRepositoryCI)

	router.OPTIONS("/users/:user", allowCors)
	router.GET("/users/:user", repohealth.GetUserPRs)

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

func allowCors(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
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
	// TODO(gracew): remove once there's a proper dev setup
	w.Header().Set("Access-Control-Allow-Origin", "*")
	bytes, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Panicln(err)
	}
	w.Write(bytes)
}
