package scraper

import (
	"fmt"
	"net/http"
	"time"

	"github.com/SyNdicateFoundation/legitagent"
	"github.com/avast/retry-go/v5"
	"github.com/corpix/uarand"
	"github.com/rs/zerolog/log"
)

func HTTPGetRequestRetryingWithRandomHeaders(url string, maxRetries int) (*http.Response, error) {
	var resp *http.Response
	err := retry.New(
		retry.Attempts(uint(maxRetries)),
		retry.Delay(time.Second),
		retry.DelayType(retry.FullJitterBackoffDelay),
		retry.OnRetry(func(attempt uint, err error) {
			log.Error().Err(err).Str("url", url).Msgf("retry attempt %d", attempt)
		}),
	).Do(func() (errReq error) {
		headers := GenerateHeaders()
		resp, errReq = HTTPGetRequestWithHeaders(url, headers)
		if errReq != nil || resp.StatusCode != http.StatusOK {
			e := log.Error().Err(errReq).Str("url", url).Any("headers", headers)
			if resp != nil {
				e = e.Str("status", resp.Status)
			}
			e.Msg("HTTP GET request failed")
			return errReq
		}

		return nil
	})
	return resp, err
}

var laGenerator = legitagent.NewGenerator()

func GenerateHeaders() map[string]string {
	agent, err := laGenerator.Generate()
	if err != nil {
		// Fallback to uarand
		log.Warn().Err(err).Msg("Failed to generate legit agent, falling back to uarand")
		return map[string]string{"User-Agent": uarand.GetRandom(), "Referer": "https://coworking.tyuiu.ru/shs/all_t/"}
	}
	defer laGenerator.ReleaseAgent(agent)

	headers := map[string]string{"User-Agent": agent.UserAgent, "Referer": "https://coworking.tyuiu.ru/shs/all_t/"}
	for k := range agent.Headers {
		headers[k] = agent.Headers.Get(k)
	}
	return headers
}

func HTTPGetRequestWithHeaders(url string, headers map[string]string) (*http.Response, error) {
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
