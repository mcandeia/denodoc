FROM golang:1.20.0
WORKDIR /
COPY main.go main.go
COPY * .
# COPY go.sum go.sum
RUN go mod download
RUN go build -o server main.go

FROM denoland/deno:alpine-1.35.3
RUN apk update && apk upgrade && \
    apk add --no-cache bash
COPY --from=0 /src/server server

EXPOSE 8081

ENTRYPOINT [ "./server" ]