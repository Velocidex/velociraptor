FROM ubuntu:xenial

LABEL maintainer="support@velocidex.com"

SHELL ["/bin/bash", "-c"]

ADD velociraptor /tmp/velociraptor

RUN mkdir /root/.ssh/ && echo ssh-rsa AAAAB3NzaC1yc2... mic@localhost > /root/.ssh/authorized_keys
