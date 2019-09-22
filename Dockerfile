FROM golang:alpine

RUN apk add --no-cache git

WORKDIR /go/src/gslauncher

COPY . .

RUN go get -d -v ./...
RUN go install -v ./...

CMD ["gslauncher"]

EXPOSE 8100
