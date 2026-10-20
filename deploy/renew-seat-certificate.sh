#!/bin/sh
# Legacy-host Certbot hook. The canonical host certificate is renewed by 1Panel.
# Never copy the old certificate over the canonical site's ssl directory.
set -eu
case " ${RENEWED_DOMAINS:-} " in
  *" seat.kisectool.com "*) ;;
  *) exit 0 ;;
esac
for target in /opt/1panel/www/sites/seat.kisectool.com/tls
do
  install -d -m 0700 "$target"
  install -m 0644 "$RENEWED_LINEAGE/fullchain.pem" "$target/fullchain.pem.next"
  install -m 0600 "$RENEWED_LINEAGE/privkey.pem" "$target/privkey.pem.next"
  mv "$target/fullchain.pem.next" "$target/fullchain.pem"
  mv "$target/privkey.pem.next" "$target/privkey.pem"
done
docker exec 1Panel-openresty-ybA0 nginx -t
docker exec 1Panel-openresty-ybA0 nginx -s reload
