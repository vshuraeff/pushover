# build context is prepared by goreleaser and contains only the povr binary
FROM alpine:3.22@sha256:14358309a308569c32bdc37e2e0e9694be33a9d99e68afb0f5ff33cc1f695dce AS certs

RUN apk add --no-cache ca-certificates && mkdir -p /data && chown 65532:65532 /data

FROM scratch

COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY povr /povr
COPY --from=certs --chown=65532:65532 /data /data

ENV PUSHOVER_CONFIG_DIR=/data
ENV HOME=/data
ENV PUSHOVER_NOTIFY_BACKEND=file:/data/notifications.jsonl

VOLUME /data

USER 65532:65532

ENTRYPOINT ["/povr"]
