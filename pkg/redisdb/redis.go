// Package redisdb provides a Redis-backed cache client that distinguishes
// "fresh" data from "stale" data via two keys per entry.
package redisdb

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

// New creates a SmartClient, verifying the connection with a ping.
// TLS is enabled automatically when the redis_tls setting is set.
func New(opts *redis.Options) (*SmartClient, error) {
	if viper.GetBool("redis_tls") {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &SmartClient{rdb: client}, nil
}

// SmartClient caches entries under two keys: "<key>:fresh" with a short TTL
// and "<key>:data" with a long TTL.
type SmartClient struct{ rdb *redis.Client }

// Get returns the cached data along with a fresh flag.
// fresh reports whether the data was refreshed within the short TTL;
// exist reports whether any data is present at all.
func (s *SmartClient) Get(ctx context.Context, key string) (data []byte, fresh bool, exist bool) {
	if err := s.rdb.Get(ctx, key+":fresh").Err(); err != nil {
		if err == redis.Nil {
			log.Trace().Str("key", key).Msg("Cache is old or does not exist")
			fresh = false
		} else {
			log.Error().Err(err).Str("key", key).Msg("Failed to get fresh redis cache")
			return nil, false, false
		}
	} else {
		log.Debug().Msg("Cache is fresh")
		fresh = true
	}

	data, err := s.rdb.Get(ctx, key+":data").Bytes()
	if err == redis.Nil {
		log.Debug().Str("key", key).Msg("Cache miss")
		return nil, false, false
	} else if err != nil {
		log.Error().Err(err).Str("key", key).Msg("Failed to get redis cache")
		return nil, false, false
	} else {
		log.Debug().Str("key", key).
			// RawJSON("data", data).
			Msg("Cache hit")
		return data, fresh, true
	}
}

// Set stores data under key with a short "fresh" TTL and a long "data" TTL.
func (s *SmartClient) Set(
	ctx context.Context, key string, data any, expirationShort, expirationLong time.Duration,
) error {
	if err := s.rdb.Set(ctx, key+":fresh", "", expirationShort).Err(); err != nil {
		return err
	}
	if err := s.rdb.Set(ctx, key+":data", data, expirationLong).Err(); err != nil {
		return err
	}
	return nil
}

// Close closes the underlying Redis connection.
func (s *SmartClient) Close() error { return s.rdb.Close() }
