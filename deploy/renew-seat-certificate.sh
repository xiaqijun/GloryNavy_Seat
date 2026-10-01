#!/bin/sh
# Certbot deploy hook. No private keys leave the edge host.
set -eu
case " ${RENEWED_DOMAINS:-} " in
  *" seat.kisectool.com "*) ;;
  *) exit 0 ;;
esac
for target in \
  /opt/1panel/www/sites/seat.kisectool.com/tls \
  /opt/1panel/www/sites/zz-glorynavy-seat/ssl
do
  install -d -m 0700 "$target"
  install -m 0644 "$RENEWED_LINEAGE/fullchain.pem" "$target/fullchain.pem.next"
  install -m 0600 "$RENEWED_LINEAGE/privkey.pem" "$target/privkey.pem.next"
  mv "$target/fullchain.pem.next" "$target/fullchain.pem"
  mv "$target/privkey.pem.next" "$target/privkey.pem"
done
docker exec 1Panel-openresty-ybA0 nginx -t
docker exec 1Panel-openresty-ybA0 nginx -s reload
