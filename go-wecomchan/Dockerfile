FROM node:20-alpine AS frontend-builder

WORKDIR /frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend ./
RUN npm run build

FROM golang:1.16.5-alpine3.13 AS gobuilder

RUN apk add --no-cache ca-certificates && \
    update-ca-certificates

WORKDIR /go/src/app
COPY go.mod go.sum ./
COPY *.go ./
RUN go build -o wecomchan

FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata && \
    update-ca-certificates && \
    cp /usr/share/zoneinfo/Asia/Shanghai /etc/localtime && \
    echo "Asia/Shanghai" > /etc/timezone

WORKDIR /root

COPY --from=gobuilder /go/src/app/wecomchan .
COPY --from=gobuilder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=frontend-builder /frontend/dist ./frontend/dist

EXPOSE 8080

CMD ["./wecomchan"]
