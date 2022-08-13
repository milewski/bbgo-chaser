FROM golang:1.19-alpine3.16 AS builder

RUN apk add make gcc libc-dev pkgconfig

WORKDIR /srv
ADD . /srv
RUN make build

FROM alpine:3.16

COPY --from=builder /srv/build/chaser-linux /usr/local/bin/bbgo
ENTRYPOINT ["/usr/local/bin/bbgo"]
CMD ["run", "--config", "/bbgo.yaml", "--no-compile"]
