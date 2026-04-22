FROM alpine:3.20 AS base
RUN apk add --no-cache ca-certificates tzdata xorriso
COPY proxctl /usr/local/bin/proxctl
ENTRYPOINT ["proxctl"]
CMD ["--help"]
