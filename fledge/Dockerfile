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

COPY . /go/src/vkubelet-service
WORKDIR /go/src/vkubelet-service
ARG BUILD_TAGS="netgo osusergo"

RUN go build -o vkubelet-service -ldflags "-s -w" *.go
RUN strip vkubelet-service
RUN cp vkubelet-service /usr/bin/vkubelet-service

FROM alpine
RUN apk add iproute2 
#	cgroup-tools
COPY --from=builder /usr/bin/vkubelet-service /usr/bin/vkubelet-service
COPY --from=builder /etc/ssl/certs/ /etc/ssl/certs
COPY ./startcni.sh /startcni.sh
COPY ./setupcontainercni.sh /setupcontainercni.sh
COPY ./shutdowncontainercni.sh /shutdowncontainercni.sh
COPY ./defaultconfig.json /usr/bin/defaultconfig.json
WORKDIR /usr/bin
ENTRYPOINT [ "./vkubelet-service" ] 
