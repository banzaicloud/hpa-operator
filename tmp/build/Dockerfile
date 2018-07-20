FROM alpine:3.6

ADD tmp/_output/bin/hpa-operator /usr/local/bin/hpa-operator

RUN adduser -D hpa-operator
USER hpa-operator
