ARG GO_VERSION=1.26.10-alpine
FROM golang:${GO_VERSION} AS build

# Stamped by the deployment pipeline. They are build ARGs rather than anything
# committed: a commit written into the tree describes the commit someone typed,
# not the commit that was built, and OPS-002 is precisely about asking a running
# service which commit it actually is.
ARG BUILD_COMMIT_SHA=unknown
ARG BUILD_TIME=unknown
ARG BUILD_VERSION=unknown

WORKDIR /workspace
COPY shared/go ./shared/go
COPY services/vendor-service ./services/vendor-service

WORKDIR /workspace/services/vendor-service
RUN go mod download
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-X cosmicforge/logistics/shared/go/serviceapp.commitSHA=${BUILD_COMMIT_SHA} \
                -X cosmicforge/logistics/shared/go/serviceapp.buildTime=${BUILD_TIME} \
                -X cosmicforge/logistics/shared/go/serviceapp.version=${BUILD_VERSION}" \
      -o /out/vendor-service ./cmd
RUN CGO_ENABLED=0 go build -trimpath -o /out/vendor-migrate ./tools/migrate

FROM alpine:3.22
RUN apk add --no-cache ca-certificates \
    && addgroup -S cosmicforge \
    && adduser -S cosmicforge -G cosmicforge

WORKDIR /srv/vendor-service
COPY --from=build /out/vendor-service /bin/vendor-service
COPY --from=build /out/vendor-migrate /bin/vendor-migrate
COPY --from=build /workspace/services/vendor-service/migrations ./migrations

USER cosmicforge
EXPOSE 8112
ENTRYPOINT ["/bin/vendor-service"]
