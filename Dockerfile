FROM golang:1.13-alpine AS builder

ARG GITHUB_TOKEN

RUN apk update && apk add --no-cache git
RUN adduser -D -g '' appuser
RUN git config --global url."https://${GITHUB_TOKEN}:x-oauth-basic@github.com/".insteadOf "https://github.com/"

WORKDIR $GOPATH/src/io.defilade/gslauncher/

COPY ./go.mod ./go.sum ./

RUN go mod download
RUN go mod verify

COPY main.go .
COPY pkg pkg

RUN GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/gslauncher

FROM alpine

COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /go/bin/gslauncher /go/bin/gslauncher

USER appuser

ENTRYPOINT ["/go/bin/gslauncher"]

EXPOSE 9443 9444
