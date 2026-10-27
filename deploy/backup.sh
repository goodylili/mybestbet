#!/bin/sh
set -eu
apk add --no-cache aws-cli >/dev/null
while true; do
  name="mybestbet-$(date -u +%Y%m%dT%H%M%SZ).sql.gz"
  pg_dump -h postgres -U mybestbet mybestbet | gzip > "/tmp/$name"
  aws s3 cp "/tmp/$name" "s3://${BACKUP_BUCKET}/${name}"
  rm -f "/tmp/$name"
  sleep 86400
done
