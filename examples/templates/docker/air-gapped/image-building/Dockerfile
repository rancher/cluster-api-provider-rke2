FROM ubuntu:20.04
RUN apt-get update  && apt-get install -y cloud-init=22.4.2-0ubuntu0~20.04.2 systemd=245.4-4ubuntu3.20 curl=7.68.0-1ubuntu2.16 --no-install-recommends\
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*
VOLUME [/var/lib/rancher]
RUN mkdir -p /opt/rke2-artifacts
COPY files/rke2-images.linux-amd64.tar.zst /opt/rke2-artifacts/
ADD files/rke2.linux-amd64.tar.gz /opt/rke2-artifacts/
COPY files/sha256sum-amd64.txt /opt/rke2-artifacts/
WORKDIR /opt
COPY files/install.sh ./
CMD ["/lib/systemd/systemd"]
