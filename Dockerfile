FROM golang:1.25-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY . .
RUN --mount=type=secret,id=COMMON_GO_MODULES_FETCH \
    git config --global url."https://x-access-token:$(cat /run/secrets/COMMON_GO_MODULES_FETCH)@github.com/kenyamaneko/overload-party-common".insteadOf "https://github.com/kenyamaneko/overload-party-common" && \
    GOPRIVATE=github.com/kenyamaneko/* go mod download
RUN CGO_ENABLED=0 GOOS=linux go build -o /shop ./cmd/server

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=builder /shop /app/shop
EXPOSE 9006
ENTRYPOINT ["/app/shop"]
