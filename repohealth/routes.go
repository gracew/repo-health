package repohealth

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/julienschmidt/httprouter"
	"github.com/machinebox/graphql"
)

func getWeeks(r *http.Request) int {
	numWeeks, err := strconv.Atoi(r.URL.Query().Get("weeks"))
	if err != nil {
		log.Println("failed to parse weeks parameter, using default of 6", err)
		return 6
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

func handleError(err error, w http.ResponseWriter) {
	log.Println(err)
	if strings.Contains(err.Error(), "Could not resolve to a Repository") {
		w.WriteHeader(http.StatusNotFound)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
}

func GetRepositoryIssues(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	client := graphql.NewClient("https://api.github.com/graphql")

	authHeader := r.Header.Get("Authorization")
	numWeeks := getWeeks(r)
	since := getStartDate(numWeeks)

	issues, err := getIssuesCreatedSince(client, authHeader, params.ByName("owner"), params.ByName("name"), since)
	if err != nil {
		handleError(err, w)
		return
	}
	issueScore := GetIssueScore(issues, since, getWeeks(r))
	json.NewEncoder(w).Encode(issueScore)
}

func GetRepositoryPRs(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	client := graphql.NewClient("https://api.github.com/graphql")

	authHeader := r.Header.Get("Authorization")
	numWeeks := getWeeks(r)
	since := getStartDate(numWeeks)

	prs, err := getRepoPRsCreatedSince(client, authHeader, params.ByName("owner"), params.ByName("name"), since, prFragment)
	if err != nil {
		handleError(err, w)
		return
	}
	prScore := GetPRScore(prs, since, numWeeks)
	json.NewEncoder(w).Encode(prScore)
}

func GetRepositoryCI(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	client := graphql.NewClient("https://api.github.com/graphql")

	authHeader := r.Header.Get("Authorization")
	numWeeks := getWeeks(r)
	since := getStartDate(numWeeks)

	prs, err := getRepoPRsCreatedSince(client, authHeader, params.ByName("owner"), params.ByName("name"), since, prWithCIMetadataFragment)
	if err != nil {
		handleError(err, w)
		return
	}
	ciScore := GetCIScore(prs, since, numWeeks)
	json.NewEncoder(w).Encode(ciScore)
}

func GetUserPRs(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
	client := graphql.NewClient("https://api.github.com/graphql")

	authHeader := r.Header.Get("Authorization")

	user := params.ByName("user")
	numWeeks := getWeeks(r)
	since := getStartDate(numWeeks)

	prs, err := getUserPRsCreatedSince(client, authHeader, user, since)
	if err != nil {
		handleError(err, w)
		return
	}
	prScore := GetPRScore(prs, since, numWeeks)
	json.NewEncoder(w).Encode(prScore)
}
