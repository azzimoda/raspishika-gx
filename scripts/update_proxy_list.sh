#!/usr/bin/env bash
mkdir -p storage
curl -sL https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/all/data.json -o storage/proxies.json
