#!/bin/sh
set -eu
# Only manage this application's dedicated nftables table.
{
  if nft list table inet glorynavy >/dev/null 2>&1; then
    printf 'delete table inet glorynavy\n'
  fi
  cat <<'RULES'
table inet glorynavy {
  chain api_input {
    type filter hook input priority -10; policy accept;
    ip daddr 10.233.53.209 tcp dport 18080 ip saddr != { 127.0.0.1, 10.233.53.17, 10.233.53.209 } reject with tcp reset
  }
}
RULES
} | nft -f -
