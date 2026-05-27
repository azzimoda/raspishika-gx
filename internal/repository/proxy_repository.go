package repository

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
)

type ProxyRepository interface {
	// UpdateProxyList updates the proxy list from the configured file
	// UpdateProxyList() error

	// All returns all proxies in the list
	All() []string

	// Next returns the next proxy from the list
	Next() (idx int, proxy string)
}

func NewProxyRepository(proxyListFile string) (ProxyRepository, error) {
	proxies, err := loadProxies(proxyListFile)
	if err != nil {
		return nil, err
	}

	return &proxyRepository{proxies: proxies}, nil
}

type proxyRepository struct {
	proxies []string
	current int
}

func (r *proxyRepository) All() []string { return r.proxies }

func (r *proxyRepository) Next() (int, string) {
	idx := r.current
	proxy := r.proxies[idx]

	if r.current >= len(r.proxies) {
		r.current = 0
	}
	r.current++

	return idx, proxy
}

func loadProxies(proxyListFile string) ([]string, error) {
	bytes, err := os.ReadFile(proxyListFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load proxy list file: %w", err)
	}

	var proxies []struct {
		Protocol    string `json:"protocol"`
		IP          string `json:"ip"`
		Port        int    `json:"port"`
		Geolocation struct {
			Country string `json:"country"`
		} `json:"geolocation"`
	}
	json.Unmarshal(bytes, &proxies)

	// Filter SOCKS and not Russian proxies
	var filteredProxies []string
	for _, proxy := range proxies {
		if proxy.Protocol == "socks5" && proxy.Geolocation.Country != "RU" {
			filteredProxies = append(filteredProxies, fmt.Sprintf("%s:%d", proxy.IP, proxy.Port))
		}
	}
	log.Debug().Any("proxies", len(filteredProxies)).Msg("Loaded proxies")
	return filteredProxies, nil
}
