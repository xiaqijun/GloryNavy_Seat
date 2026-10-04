#!/bin/bash
# Run as root after uploading and verifying a release, and provisioning secrets.
set -euo pipefail
version=${1:?Usage: activate-release.sh v0.1.0[-suffix]}
[[ "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9.-]+)?$ ]] || exit 2
release=/opt/glorynavy/releases/$version
test -d "$release"
test -s /etc/glorynavy/seat.env
(cd "$release" && test -s release.json)
source_branch=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["source_branch"])' "$release/release.json")
source_dirty=$(python3 -c 'import json,sys; print(str(json.load(open(sys.argv[1]))["source_dirty"]).lower())' "$release/release.json")
if [[ "$source_branch" != "main" || "$source_dirty" != "false" ]]; then
  printf 'Refusing production activation: release must be built from a clean main branch (branch=%s dirty=%s)\n' "$source_branch" "$source_dirty" >&2
  exit 3
fi
(cd "$release" && sha256sum --quiet -c SHA256SUMS)
chmod 0755 "$release"/bin/*
install -m 0644 "$release/deploy/compose.database.yaml" /etc/glorynavy/compose.database.yaml
install -m 0755 "$release/deploy/api-firewall.sh" /etc/glorynavy/api-firewall.sh
for unit in glorynavy glorynavy-database glorynavy-network; do
  install -m 0644 "$release/deploy/$unit.service" "/etc/systemd/system/$unit.service"
done
systemctl daemon-reload
systemctl enable --now glorynavy-database.service glorynavy-network.service
python3 "$release/deploy/provision-database.py"
systemctl stop glorynavy.service
install -d -m 0700 /var/backups/glorynavy
stamp=$(date -u +%Y%m%dT%H%M%SZ)
umask 077
docker compose -f /etc/glorynavy/compose.database.yaml exec -T db \
  pg_dump -U postgres -d glorynavy -Fc > "/var/backups/glorynavy/before-$version-$stamp.dump"
cp /etc/glorynavy/seat.env "/var/backups/glorynavy/before-$version-$stamp.env"
ln -s "$release" /opt/glorynavy/current.next
mv -Tf /opt/glorynavy/current.next /opt/glorynavy/current
systemctl enable --now glorynavy.service
for attempt in $(seq 1 30); do
  if curl --fail --silent http://10.233.53.209:18080/health/ready >/dev/null; then
    printf 'Release %s is ready\n' "$version"
    exit 0
  fi
  sleep 2
done
printf 'Release did not become ready; inspect journalctl -u glorynavy. Do not blindly downgrade schema.\n' >&2
exit 1
