#!/bin/bash
# Run as root on the production edge host after uploading a release tarball.
set -euo pipefail
version=${1:?Usage: activate-edge-release.sh v0.1.0[-suffix]}
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] || exit 2
site=${SITE_ROOT:-/opt/1panel/www/sites/seat.kisectool.com}
openresty_container=${OPENRESTY_CONTAINER:-1Panel-openresty-ybA0}
release="$site/releases/$version"
test -d "$release"
test -s "$release/release.json"
source_branch=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("source_branch", ""))' "$release/release.json")
source_dirty=$(python3 -c 'import json,sys; value=json.load(open(sys.argv[1])).get("source_dirty"); print("unknown" if value is None else str(value).lower())' "$release/release.json")
if [[ -z "$source_branch" || "$source_dirty" == "unknown" ]]; then
  if [[ "${ALLOW_LEGACY_RELEASE:-}" != "1" ]]; then
    printf 'Refusing edge activation: release has no main-branch provenance; set ALLOW_LEGACY_RELEASE=1 only for an approved historical rollback\n' >&2
    exit 3
  fi
  printf 'Warning: activating historical edge release without branch provenance (explicit rollback override)\n' >&2
elif [[ "$source_branch" != "main" || "$source_dirty" != "false" ]]; then
  printf 'Refusing edge activation: release must be built from a clean main branch (branch=%s dirty=%s)\n' "$source_branch" "$source_dirty" >&2
  exit 3
fi
(cd "$release" && sha256sum --quiet -c SHA256SUMS)
test -s "$release/web/index.html"
ln -sfn "releases/$version" "$site/current.next"
mv -Tf "$site/current.next" "$site/current"
docker exec "$openresty_container" test -f /www/sites/zz-glorynavy-seat/index/index.html
docker exec "$openresty_container" nginx -t >/dev/null
code=$(curl -ksS -o /dev/null -w '%{http_code}' https://seat.kisectool.com/)
test "$code" = 200
echo "EDGE_RELEASE_READY:$version"
echo "PUBLIC_HOME:$code"
