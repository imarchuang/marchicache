FROM golang:1.22-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY . .
RUN CGO_ENABLED=0 go build -o /marchicache ./cmd/marchicache

FROM alpine:3.20
COPY --from=build /marchicache /usr/local/bin/marchicache
EXPOSE 6380
ENTRYPOINT ["marchicache"]
