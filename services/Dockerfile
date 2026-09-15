ARG GO_VERSION=1.26-alpine
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
RUN go build \
      -ldflags "-X cosmicforge/logistics/shared/go/serviceapp.commitSHA=${BUILD_COMMIT_SHA} \
                -X cosmicforge/logistics/shared/go/serviceapp.buildTime=${BUILD_TIME} \
                -X cosmicforge/logistics/shared/go/serviceapp.version=${BUILD_VERSION}" \
      -o /bin/cosmicforge-logistics-service ./cmd

FROM alpine:3.22
RUN addgroup -S cosmicforge && adduser -S cosmicforge -G cosmicforge
USER cosmicforge
COPY --from=build /bin/cosmicforge-logistics-service /bin/cosmicforge-logistics-service
EXPOSE 8080
ENTRYPOINT ["/bin/cosmicforge-logistics-service"]
