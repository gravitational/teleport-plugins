package slack

import (
	"net/http"

	"github.com/go-resty/resty/v2"
)

func makeSlackClient() *resty.Client {
	return resty.
		NewWithClient(&http.Client{
			Timeout: slackHTTPTimeout,
			Transport: &http.Transport{
				MaxConnsPerHost:     slackMaxConns,
				MaxIdleConnsPerHost: slackMaxConns,
			},
		}).
		SetHeader("Content-Type", "application/json").
		SetHeader("Accept", "application/json")
}
