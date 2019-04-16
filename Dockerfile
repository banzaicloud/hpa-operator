FROM golang:1.9-alpine

ADD . /go/src/github.com/banzaicloud/hpa-operator
WORKDIR /go/src/github.com/banzaicloud/hpa-operator
RUN GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/hpa-operator cmd/hpa-operator/main.go

FROM alpine:3.6

COPY --from=0 /tmp/hpa-operator /usr/local/bin/hpa-operator
RUN apk update && apk add ca-certificates

USER 65534:65534

ENTRYPOINT ["/usr/local/bin/hpa-operator"]