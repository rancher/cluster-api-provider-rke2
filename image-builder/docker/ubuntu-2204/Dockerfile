FROM ubuntu:20.04

LABEL org.opencontainers.image.source https://github.com/rancher-sandbox/cluster-api-provider-rke2

ARG RKE2_VERSION

# hadolint global ignore=DL3008
RUN apt-get update  && apt-get install -y cloud-init systemd curl --no-install-recommends \
&& apt-get clean \
&& rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/rke2-artifacts && \
curl -sfL -o /opt/rke2-artifacts/rke2-images.linux-amd64.tar.zst https://github.com/rancher/rke2/releases/download/v${RKE2_VERSION}/rke2-images.linux-amd64.tar.zst && \
curl -sfL -o /opt/rke2-artifacts/rke2.linux-amd64.tar.gz https://github.com/rancher/rke2/releases/download/v${RKE2_VERSION}/rke2.linux-amd64.tar.gz && \
curl -sfL -o /opt/rke2-artifacts/sha256sum-amd64.txt https://github.com/rancher/rke2/releases/download/v${RKE2_VERSION}/sha256sum-amd64.txt && \
curl -sfL -o /opt/install.sh https://get.rke2.io

VOLUME ["/var/lib/rancher"]

CMD ["/lib/systemd/systemd"]
