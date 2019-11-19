FROM golang:alpine as builder
ENV PATH /go/bin:/usr/local/go/bin:$PATH
ENV GOPATH /go

RUN apk add --no-cache \
	ca-certificates \
	--virtual .build-deps \
	git \
	gcc \
	libc-dev \
	libgcc \
	make \
	bash \
	binutils

COPY . /go/src/fledge/fledge-integrated
WORKDIR /go/src/fledge/fledge-integrated
ARG BUILD_TAGS="netgo osusergo"

RUN go build -o fledge-integrated -ldflags "-s -w" *.go
RUN strip fledge-integrated
RUN cp fledge-integrated /usr/bin/fledge-integrated

FROM alpine:edge
RUN apk update && apk add iproute2 \
	containerd
#	cgroup-tools
COPY --from=builder /usr/bin/fledge-integrated /fledge-integrated
COPY --from=builder /etc/ssl/certs/ /etc/ssl/certs
COPY ./startcni.sh /startcni.sh
COPY ./setupcontainercni.sh /setupcontainercni.sh
COPY ./shutdowncontainercni.sh /shutdowncontainercni.sh
COPY ./defaultconfig.json /usr/bin/defaultconfig.json

ENTRYPOINT [ "./fledge-integrated" ] 
