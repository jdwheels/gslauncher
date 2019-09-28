FROM golang:alpine AS builder

RUN apk update && apk add --no-cache git
RUN adduser -D -g '' appuser

WORKDIR $GOPATH/src/io.defilade/gslauncher/

COPY go.mod .
COPY go.sum .
COPY main.go .

RUN go mod download
RUN go mod verify
RUN GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /go/bin/gslauncher

FROM alpine

COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /go/bin/gslauncher /go/bin/gslauncher

USER appuser

ENTRYPOINT ["/go/bin/gslauncher"]

EXPOSE 9443
