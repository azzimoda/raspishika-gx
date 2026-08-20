package repository

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type ProxyRepository interface {
	UpdateFromJSON(data []byte) error

	// All returns all proxies in the list
	All() []string

	// Next returns the next proxy from the list
	Next() (idx int, proxy string)

	UpdatedAt() time.Time
}

func NewProxyRepository() ProxyRepository {
	return &proxyRepository{proxies: make([]string, 0)}
}

type proxyRepository struct {
	proxies   []string
	updatedAt time.Time
	current   int
}

func (r *proxyRepository) All() []string { return r.proxies }

var muNext sync.Mutex

func (r *proxyRepository) Next() (int, string) {
	muNext.Lock()
	defer muNext.Unlock()

	if len(r.proxies) == 0 {
		return 0, ""
	}

	if r.current >= len(r.proxies) {
		r.current = 0
	}

	idx := r.current
	proxy := r.proxies[idx]
	r.current++

	return idx, proxy
}

var muUpdate sync.Mutex

// UpdateFromJSON parses and stores the proxy list. On failure (malformed data
// or no usable proxies) the previous list is kept and an error is returned, so
// the caller can retry instead of caching an empty list.
func (r *proxyRepository) UpdateFromJSON(data []byte) error {
	muUpdate.Lock()
	defer muUpdate.Unlock()

	var proxies []struct {
		Protocol    string `json:"protocol"`
		IP          string `json:"ip"`
		Port        int    `json:"port"`
		Geolocation struct {
			Country string `json:"country"`
		} `json:"geolocation"`
	}
	if err := json.Unmarshal(data, &proxies); err != nil {
		return fmt.Errorf("failed to parse proxy list: %w", err)
	}

	// Filter SOCKS and not Russian proxies
	var filteredProxies []string
	for _, proxy := range proxies {
		if proxy.Protocol == "socks5" && proxy.IP != "" && proxy.Port > 0 && proxy.Geolocation.Country != "RU" {
			filteredProxies = append(filteredProxies, fmt.Sprintf("%s:%d", proxy.IP, proxy.Port))
		}
	}
	if len(filteredProxies) == 0 {
		return fmt.Errorf("no usable proxies in list")
	}
	log.Debug().Any("proxies", len(filteredProxies)).Msg("Loaded proxies")

	r.proxies = filteredProxies
	r.current = 0
	r.updatedAt = time.Now()
	return nil
}

func (r *proxyRepository) UpdatedAt() time.Time { return r.updatedAt }
