FROM quay.io/fedora/fedora:42
ARG TARGETPLATFORM

COPY --chown=0:0 --chmod=0755 $TARGETPLATFORM/leaktk /usr/local/bin/leaktk

RUN \
  dnf install -y --setopt=tsflags=nodocs git && \
  dnf clean all -y --enablerepo='*' && \
  mkdir -p /var/lib/leaktk && \
  useradd -u 1001 -r -g 0 -d /var/lib/leaktk -c "LeakTK" leaktk && \
  chown -R 1001:0 /var/lib/leaktk && \
  chmod -R ug=rwX,o-rwx /var/lib/leaktk

USER 1001
ENTRYPOINT ["/usr/local/bin/leaktk"]
