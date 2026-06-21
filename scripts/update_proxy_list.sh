#!/usr/bin/env bash
FILE="storage/proxies.json"
mkdir -p storage
curl -sL https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/all/data.json -o "$FILE"
echo "Proxy list updated: $FILE"
