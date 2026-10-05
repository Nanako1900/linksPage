# syntax=docker/dockerfile:1
# Backup sidecar: pg_dump + tar, optionally encrypted with age.
FROM postgres:18-alpine
RUN apk add --no-cache age tini
COPY backup.sh /usr/local/bin/linkspage-backup
RUN chmod 0755 /usr/local/bin/linkspage-backup
ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/linkspage-backup"]
CMD ["loop"]
