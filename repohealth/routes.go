package repohealth

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/machinebox/graphql"
)

func getWeeks(r *http.Request) int {
	queryValues := r.URL.Query()
	numWeeks, err := strconv.Atoi(queryValues.Get("weeks"))
	if err != nil {
		log.Panicln(err)
	}
	return numWeeks
}

// returns the first Sunday after (today - numWeeks)
func getStartDate(numWeeks int) time.Time {
	since := time.Now().AddDate(0, 0, -7*numWeeks)
	for since.Weekday() != 0 {
		since = since.AddDate(0, 0, 1)
	}
	return since
}

func GetRepositoryIssues(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	client := graphql.NewClient("https://api.github.com/graphql")

	// TODO(gracew): if there's no auth header then return a 403...
	authHeader := r.Header.Get("Authorization")
	numWeeks := getWeeks(r)
	since := getStartDate(numWeeks)

	issues := getIssuesCreatedSince(client, authHeader, params.ByName("owner"), params.ByName("name"), since)
	issueScore := GetIssueScore(client, authHeader, params.ByName("owner"), params.ByName("name"), getWeeks(r))
	// TODO(gracew): remove once there's a proper dev setup
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(issueScore)
}

func GetRepositoryPRs(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	client := graphql.NewClient("https://api.github.com/graphql")

	authHeader := r.Header.Get("Authorization")
	numWeeks := getWeeks(r)
	since := getStartDate(numWeeks)

	prs := getRepoPRsCreatedSince(client, authHeader, params.ByName("owner"), params.ByName("name"), since, prFragment)
	prScore := GetPRScore(prs, since, numWeeks)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(prScore)
}

func GetRepositoryCI(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	client := graphql.NewClient("https://api.github.com/graphql")

	authHeader := r.Header.Get("Authorization")
	numWeeks := getWeeks(r)
	since := getStartDate(numWeeks)

	prs := getRepoPRsCreatedSince(client, authHeader, params.ByName("owner"), params.ByName("name"), since, prWithCIMetadataFragment)
	ciScore := GetCIScore(prs, since, numWeeks)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(ciScore)
}

func GetUserPRs(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	client := graphql.NewClient("https://api.github.com/graphql")

	authHeader := r.Header.Get("Authorization")

	user := params.ByName("user")
	numWeeks := getWeeks(r)
	since := getStartDate(numWeeks)

	prs := getUserPRsCreatedSince(client, authHeader, user, since)
	prScore := GetPRScore(prs, since, numWeeks)
	// TODO(gracew): remove once there's a proper dev setup
	w.Header().Set("Access-Control-Allow-Origin", "*")
	json.NewEncoder(w).Encode(prScore)
}
