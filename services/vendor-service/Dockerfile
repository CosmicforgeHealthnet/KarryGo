ARG GO_VERSION=1.26-alpine
FROM golang:${GO_VERSION} AS build

WORKDIR /workspace
COPY shared/go ./shared/go
COPY services/vendor-service ./services/vendor-service

WORKDIR /workspace/services/vendor-service
RUN go mod download
RUN CGO_ENABLED=0 go build -trimpath -o /out/vendor-service ./cmd
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
