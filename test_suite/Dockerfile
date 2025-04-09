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

# Python requirements
COPY --from=get-requirements /common/test_suite/python_requirements.txt /go/python_requirements.txt
RUN apt-get -y install python3-pip &&\
    python3 -m pip config set global.break-system-packages true &&\
    pip install -r python_requirements.txt

# Go packages
COPY --from=get-requirements /common/go.mod /go/go.mod
RUN go mod download

# Clone repository, prevent cache from using old version by adding current version to Dockerfile
ADD https://api.github.com/repos/eduP2P/common/git/refs/heads/${BRANCH} version.json
RUN git clone -b ${BRANCH} --single-branch https://github.com/eduP2P/common

# Build eduP2P binaries
WORKDIR /go/common/test_suite
RUN for binary in test_client control_server relay_server; do go build -o $binary/$binary $binary/*.go; done

ENTRYPOINT ["/go/common/test_suite/system_tests.sh"]