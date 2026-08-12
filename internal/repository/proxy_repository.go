package repository

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

type ProxyRepository interface {
	UpdateFromJSON(data []byte)

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

	idx := r.current
	proxy := r.proxies[idx]

	if r.current >= len(r.proxies) {
		r.current = 0
	}
	r.current++

	return idx, proxy
}

var muUpdate sync.Mutex

func (r *proxyRepository) UpdateFromJSON(data []byte) {
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
	json.Unmarshal(data, &proxies)

	// Filter SOCKS and not Russian proxies
	var filteredProxies []string
	for _, proxy := range proxies {
		if proxy.Protocol == "socks5" && proxy.Geolocation.Country != "RU" {
			filteredProxies = append(filteredProxies, fmt.Sprintf("%s:%d", proxy.IP, proxy.Port))
		}
	}
	log.Debug().Any("proxies", len(filteredProxies)).Msg("Loaded proxies")

	r.proxies = filteredProxies
	r.updatedAt = time.Now()
}

func (r *proxyRepository) UpdatedAt() time.Time { return r.updatedAt }
