FROM golang:1.22

# Add latest repo version to container to prevent Docker from caching the git clone command
ADD https://api.github.com/repos/eduP2P/common/git/refs/heads/main version.json
RUN git clone https://github.com/eduP2P/common

WORKDIR /go/common/test_suite

# Commandline dependencies
RUN apt-get update &&\
    apt-get -y install sudo &&\
    xargs -a system_test_requirements.txt sudo apt-get -y install

# Python dependencies
RUN apt-get -y install python3-pip &&\
    python3 -m pip config set global.break-system-packages true &&\
    pip install -r python_requirements.txt

# Build eduP2P binaries
RUN for binary in test_client control_server relay_server; do go build -o $binary/$binary $binary/*.go; done

ENTRYPOINT ["/go/common/test_suite/system_tests.sh"]