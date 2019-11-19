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
	bash 

COPY . /go/src/vkube-manager-service
WORKDIR /go/src/vkube-manager-service
ARG BUILD_TAGS="netgo osusergo"

RUN go build -o vkubelet-server -ldflags "-w" -buildmode=pie *.go
RUN cp vkubelet-server /usr/bin/vkubelet-server

FROM alpine
RUN echo "http://dl-cdn.alpinelinux.org/alpine/latest-stable/community" >> /etc/apk/repositories
RUN apk update
RUN apk add docker

COPY --from=builder /usr/bin/vkubelet-server /usr/bin/vkubelet-server
COPY ./defaultPod.json /usr/bin/defaultPod.json
COPY --from=builder /etc/ssl/certs/ /etc/ssl/certs

WORKDIR /usr/bin
ENTRYPOINT [ "./vkubelet-server", "127.0.0.1", "6443", "defaultPod.json" ] 
