ARG GO_VERSION=1.26-alpine
FROM golang:${GO_VERSION} AS build

ARG SERVICE_DIR
WORKDIR /workspace

COPY shared/go ./shared/go
COPY services/${SERVICE_DIR} ./services/${SERVICE_DIR}

WORKDIR /workspace/services/${SERVICE_DIR}
RUN go mod download
RUN go build -o /bin/cosmicforge-logistics-service ./cmd

FROM alpine:3.22
RUN addgroup -S cosmicforge && adduser -S cosmicforge -G cosmicforge
USER cosmicforge
COPY --from=build /bin/cosmicforge-logistics-service /bin/cosmicforge-logistics-service
EXPOSE 8080
ENTRYPOINT ["/bin/cosmicforge-logistics-service"]

