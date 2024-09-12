FROM golang:1.22

# Dependencies
RUN apt-get update &&\
    apt-get -y install git iproute2 iputils-ping sed sudo wireguard

RUN git clone https://github.com/hb140502/eduP2P_test_suite

# TODO remove before commit
RUN rm -r ./eduP2P_test_suite/cmd/toverstok/*
COPY . ./eduP2P_test_suite/cmd/toverstok

WORKDIR ./eduP2P_test_suite/cmd/toverstok/

RUN go build -o toverstok *.go

ENTRYPOINT ["/go/eduP2P_test_suite/cmd/toverstok/setup_toverstok.sh"]