FROM golang:1.22

# Dependencies
RUN apt-get update &&\
    apt-get -y install git iproute2 iputils-ping sed wireguard

# Add latest repo version to container to prevent Docker from caching the git clone command
ADD https://api.github.com/repos/hb140502/eduP2P_test_suite/git/refs/heads/main version.json
RUN git clone https://github.com/hb140502/eduP2P_test_suite

WORKDIR /go/eduP2P_test_suite/cmd/toverstok/

RUN go build -o toverstok *.go

ENTRYPOINT ["/go/eduP2P_test_suite/cmd/toverstok/setup_client.sh"]