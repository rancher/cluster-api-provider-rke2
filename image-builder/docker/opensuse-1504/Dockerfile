FROM opensuse/leap:15.4

LABEL org.opencontainers.image.source https://github.com/rancher-sandbox/cluster-api-provider-rke2

ARG RKE2_VERSION

ENV container=docker

RUN zypper install -y dbus-1 systemd-sysvinit && \
    cd /usr/lib/systemd/system/sysinit.target.wants/; \
    for i in *; do [ $i = systemd-tmpfiles-setup.service ] || rm -f $i; done ; \
    rm -f /usr/lib/systemd/system/multi-user.target.wants/* ; \
    rm -f /etc/systemd/system/*.wants/* ; \
    rm -f /usr/lib/systemd/system/local-fs.target.wants/* ; \
    rm -f /usr/lib/systemd/system/sockets.target.wants/*udev* ; \
    rm -f /usr/lib/systemd/system/sockets.target.wants/*initctl* ; \
    rm -f /usr/lib/systemd/system/basic.target.wants/* ; \
    rm -f /usr/lib/systemd/system/anaconda.target.wants/*

RUN mkdir -p /opt/rke2-artifacts && \
    curl -sfL -o /opt/rke2-artifacts/rke2-images.linux-amd64.tar.zst https://github.com/rancher/rke2/releases/download/v${RKE2_VERSION}/rke2-images.linux-amd64.tar.zst && \
    curl -sfL -o /opt/rke2-artifacts/rke2.linux-amd64.tar.gz https://github.com/rancher/rke2/releases/download/v${RKE2_VERSION}/rke2.linux-amd64.tar.gz && \
    curl -sfL -o /opt/rke2-artifacts/sha256sum-amd64.txt https://github.com/rancher/rke2/releases/download/v${RKE2_VERSION}/sha256sum-amd64.txt && \
    curl -sfL -o /opt/install.sh https://get.rke2.io

VOLUME ["/sys/fs/cgroup"]

VOLUME ["/var/lib/rancher"]

CMD ["/sbin/init"]
