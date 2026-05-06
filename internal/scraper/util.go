package scraper

import (
	"fmt"
	"net/http"
	"time"

	"github.com/SyNdicateFoundation/legitagent"
	"github.com/corpix/uarand"
	"github.com/rs/zerolog/log"
)

func HTTPGetRequestRetryingRandomHeaders(url string, maxRetries int) (*http.Response, error) {
	retries := 0
	for retries < maxRetries {
		headers := GenerateHeaders()
		resp, err := HTTPGetRequestHeaders(url, headers)
		if err == nil && resp.StatusCode == 200 {
			log.Trace().Str("url", url).Int("statusCode", resp.StatusCode).Msg("HTTP GET request succeeded")
			return resp, nil
		}

		e := log.Error().Err(err).Str("url", url).Any("headers", headers)
		if resp != nil {
			e = e.Str("status", resp.Status)
		}
		e.Msg("HTTP GET request failed")

		retries++
		time.Sleep(time.Duration(retries) * time.Second)
	}
	log.Error().Str("url", url).Msgf("HTTP GET request failed after %d retries", maxRetries)
	return nil, fmt.Errorf("failed to get %s after %d retries", url, maxRetries)
}

var g = legitagent.NewGenerator()

func GenerateHeaders() map[string]string {
	agent, err := g.Generate()
	if err != nil {
		// Fallback to uarand
		log.Warn().Err(err).Msg("Failed to generate legit agent, falling back to uarand")
		return map[string]string{"User-Agent": uarand.GetRandom(), "Referer": "https://coworking.tyuiu.ru/shs/all_t/"}
	}
	defer g.ReleaseAgent(agent)

	headers := map[string]string{
		"User-Agent": agent.UserAgent,
		"Referer":    "https://coworking.tyuiu.ru/shs/all_t/",
	}
	for k := range agent.Headers {
		headers[k] = agent.Headers.Get(k)
	}
	return headers
}

func HTTPGetRequestHeaders(url string, headers map[string]string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, value := range headers {
		req.Header.Add(key, value)
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}
