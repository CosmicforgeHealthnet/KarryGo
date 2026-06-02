ARG GO_VERSION=1.26-alpine
FROM golang:${GO_VERSION} AS build

ARG SERVICE_DIR
WORKDIR /workspace

COPY shared/go ./shared/go
COPY services/${SERVICE_DIR} ./services/${SERVICE_DIR}

WORKDIR /workspace/services/${SERVICE_DIR}
RUN go mod download
RUN go build -o /bin/karrygo-service ./cmd

FROM alpine:3.22
RUN addgroup -S karrygo && adduser -S karrygo -G karrygo
USER karrygo
COPY --from=build /bin/karrygo-service /bin/karrygo-service
EXPOSE 8080
ENTRYPOINT ["/bin/karrygo-service"]

