FROM golang:1.21.0 AS builder
ARG CGO_ENABLED=0
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build ./cmd/place/main.go


#FROM scratch
FROM busybox

EXPOSE 8080

COPY crontab /var/spool/cron/crontabs/root
COPY --from=builder /app/main /
COPY --from=builder /app/web/ /web
COPY --from=builder /app/*.sh /

RUN chmod +x /*.sh

ENTRYPOINT ["/entrypoint.sh"]
