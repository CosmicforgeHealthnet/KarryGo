ARG GO_VERSION=1.26-alpine
FROM golang:${GO_VERSION} AS build

WORKDIR /workspace

COPY shared/go ./shared/go
COPY services/driver-dispatch-delivery-service ./services/driver-dispatch-delivery-service

WORKDIR /workspace/services/driver-dispatch-delivery-service
RUN go mod download
RUN go build -o /bin/driver-dispatch-delivery-service ./cmd

FROM alpine:3.22

RUN addgroup -S karrygo && adduser -S karrygo -G karrygo
USER karrygo

COPY --from=build /bin/driver-dispatch-delivery-service /bin/driver-dispatch-delivery-service

EXPOSE 8103
ENTRYPOINT ["/bin/driver-dispatch-delivery-service"]
