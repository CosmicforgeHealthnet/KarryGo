ARG GO_VERSION=1.26.11-alpine
FROM golang:${GO_VERSION} AS build

ARG BUILD_COMMIT_SHA=unknown
ARG BUILD_TIME=unknown
ARG BUILD_VERSION=unknown

WORKDIR /workspace
COPY shared/go ./shared/go
COPY services/fleet-partner-service ./services/fleet-partner-service

WORKDIR /workspace/services/fleet-partner-service
RUN go mod download
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-X cosmicforge/logistics/shared/go/serviceapp.commitSHA=${BUILD_COMMIT_SHA} \
                -X cosmicforge/logistics/shared/go/serviceapp.buildTime=${BUILD_TIME} \
                -X cosmicforge/logistics/shared/go/serviceapp.version=${BUILD_VERSION}" \
      -o /out/fleet-partner-service ./cmd
RUN CGO_ENABLED=0 go build -trimpath -o /out/fleet-partner-worker ./cmd/worker

FROM alpine:3.22
# The alpine:3.22 base still ships OpenSSL 3.5.9-r0; CVE-2026-124709 is fixed
# in 3.5.9-r2 (Alpine 3.22 main). Upgrade the two runtime libraries and fail
# the build if they are still below the fixed release, printing the installed
# versions as evidence.
RUN apk add --no-cache ca-certificates wget \
    && apk upgrade --no-cache libssl3 libcrypto3 \
    && for p in libssl3 libcrypto3; do \
         v="$(apk info -e -v "$p" | sed "s/^$p-//")"; \
         [ "$(apk version -t "$v" 3.5.9-r2)" != "<" ] || { echo "$p $v is below 3.5.9-r2"; exit 1; }; \
         echo "openssl-runtime $p $v"; \
       done \
    && addgroup -S cosmicforge \
    && adduser -S cosmicforge -G cosmicforge

WORKDIR /srv/fleet-partner-service
COPY --from=build /out/fleet-partner-service /bin/fleet-partner-service
COPY --from=build /out/fleet-partner-worker /bin/fleet-partner-worker
COPY --from=build /workspace/services/fleet-partner-service/migrations ./migrations

USER cosmicforge
EXPOSE 8113
ENTRYPOINT ["/bin/fleet-partner-service"]