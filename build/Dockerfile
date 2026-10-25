FROM golang:alpine
LABEL maintainer="Kadeem Hassam <github@peckypanda.com>"
LABEL service="zboard"

ENV HOME /tmp/

RUN \
apk add \
build-base \
git \
make \
sed \
util-linux

ENTRYPOINT ["go"]
