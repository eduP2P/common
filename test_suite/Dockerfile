# Git branch
ARG BRANCH="main"

# Stage that clones repository, to be copied by next stage for cache optimization
FROM alpine AS get-requirements
ARG BRANCH

# Clone repository, prevent cache from using old version by adding current version to Dockerfile
RUN apk add --no-cache git 
ADD https://api.github.com/repos/eduP2P/common/git/refs/heads/${BRANCH} version.json
RUN git clone -b ${BRANCH} --single-branch https://github.com/eduP2P/common

# Stage that makes optimal use of cache by first copying and installing requirements from previous stage, and only cloning repository afterwards
FROM golang:1.22 AS run-system-tests
ARG BRANCH

# Command-line requirements
COPY --from=get-requirements /common/test_suite/system_test_requirements.txt /go/system_test_requirements.txt
RUN apt-get update &&\
    apt-get -y install sudo &&\
    xargs -a system_test_requirements.txt sudo apt-get -y install

# Go packages
COPY --from=get-requirements /common/go.mod /go/src/go.mod
WORKDIR /go/src
RUN go mod download 

# Clone repository, prevent cache from using old version by adding current version to Dockerfile
WORKDIR /go
ADD https://api.github.com/repos/eduP2P/common/git/refs/heads/${BRANCH} version.json
RUN git clone -b ${BRANCH} --single-branch https://github.com/eduP2P/common

# Build eduP2P binaries
WORKDIR /go/common/test_suite
RUN for binary in test_client control_server relay_server; do go build -o $binary/$binary $binary/*.go; done

ENTRYPOINT ["/go/common/test_suite/system_tests.sh"]