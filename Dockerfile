FROM golang:1.20.0
WORKDIR /
COPY cmd cmd
COPY pkg pkg
COPY * .
RUN go mod download
RUN go build -o server /cmd/main.go

FROM denoland/deno:alpine-1.35.3
RUN apk update && apk upgrade && \
    apk add --no-cache bash
COPY --from=0 /server ./server

EXPOSE 8080

ENTRYPOINT [ "/server" ]