ARG GO_VERSION=1.26.11-alpine
FROM golang:${GO_VERSION} AS build

ARG SERVICE_DIR
# Stamped by the deployment pipeline. They are build ARGs rather than anything
# committed: a commit written into the tree describes the commit someone typed,
# not the commit that was built, and OPS-002 is precisely about asking a running
# service which commit it actually is.
ARG BUILD_COMMIT_SHA=unknown
ARG BUILD_TIME=unknown
ARG BUILD_VERSION=unknown

WORKDIR /workspace

COPY shared/go ./shared/go
COPY services/${SERVICE_DIR} ./services/${SERVICE_DIR}

WORKDIR /workspace/services/${SERVICE_DIR}
RUN go mod download
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-X cosmicforge/logistics/shared/go/serviceapp.commitSHA=${BUILD_COMMIT_SHA} \
                -X cosmicforge/logistics/shared/go/serviceapp.buildTime=${BUILD_TIME} \
                -X cosmicforge/logistics/shared/go/serviceapp.version=${BUILD_VERSION}" \
      -o /bin/cosmicforge-logistics-service ./cmd
# Not every service has migrations or seeds. Creating the folders here means
# the COPY lines below never fail on a service that has none.
RUN mkdir -p migrations seeds

FROM alpine:3.22
# Build ARGs do not cross stages; the COPY lines below need it again.
ARG SERVICE_DIR
# ca-certificates: outbound HTTPS (Paystack, Firebase, Google) fails without it.
# wget: the container health check calls /ready with it.
# postgresql16-client: deploy/platform/bin/migrate.sh applies migrations with psql
#   for the services that have no migration runner of their own.
# The alpine:3.22 base still ships OpenSSL 3.5.9-r0; CVE-2026-124709 is fixed
# in 3.5.9-r2 (Alpine 3.22 main). Upgrade the two runtime libraries and fail
# the build if they are still below the fixed release, printing the installed
# versions as evidence.
RUN apk add --no-cache ca-certificates wget postgresql16-client \
    && apk upgrade --no-cache libssl3 libcrypto3 \
    && for p in libssl3 libcrypto3; do \
         v="$(apk info -e -v "$p" | sed "s/^$p-//")"; \
         [ "$(apk version -t "$v" 3.5.9-r2)" != "<" ] || { echo "$p $v is below 3.5.9-r2"; exit 1; }; \
         echo "openssl-runtime $p $v"; \
       done \
    && addgroup -S cosmicforge \
    && adduser -S cosmicforge -G cosmicforge

# Services read migrations/ and seeds/ relative to their working directory, so
# both live next to each other here and the process starts in this folder.
WORKDIR /srv/app
COPY --from=build /bin/cosmicforge-logistics-service /bin/cosmicforge-logistics-service
COPY --from=build /workspace/services/${SERVICE_DIR}/migrations ./migrations
COPY --from=build /workspace/services/${SERVICE_DIR}/seeds ./seeds
COPY deploy/platform/bin/migrate.sh /srv/app/migrate.sh

USER cosmicforge
EXPOSE 8080
ENTRYPOINT ["/bin/cosmicforge-logistics-service"]
