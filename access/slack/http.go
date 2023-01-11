package slack

import (
	"net/http"

	"github.com/go-resty/resty/v2"
)

const slackAPIURL = "https://slack.com/api/"

func makeSlackClient(apiURL string) *resty.Client {
	return resty.
		NewWithClient(&http.Client{
			Timeout: slackHTTPTimeout,
			Transport: &http.Transport{
				MaxConnsPerHost:     slackMaxConns,
				MaxIdleConnsPerHost: slackMaxConns,
			},
		}).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json").
		SetHostURL(apiURL)
}
