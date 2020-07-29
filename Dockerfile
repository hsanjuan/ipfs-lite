FROM golang:1.14.4-buster
LABEL maintainer="Alok Nerurkar <alok@stream.space>"

# Install deps
RUN apt-get update && apt-get install -y \
   libssl-dev \
   ca-certificates

ENV SRC_DIR /ss-ipfs-lite
ENV EXEC_DIR /ss-ipfs-lite/examples/litepeer/
ENV AUTO_DIR /ss-ipfs-lite/examples/auto/

ARG JOB=qa

# Download packages first so they can be cached.
COPY go.mod go.sum $SRC_DIR/
RUN cd $SRC_DIR \
  && go mod download

COPY . $SRC_DIR

RUN cd $EXEC_DIR && go build -o ss-light-linux-amd64
RUN cd $AUTO_DIR && go build -o auto

# Get su-exec, a very minimal tool for dropping privileges,
# and tini, a very minimal init daemon for containers
ENV SUEXEC_VERSION v0.2
ENV TINI_VERSION v0.19.0
RUN set -eux; \
   dpkgArch="$(dpkg --print-architecture)"; \
   case "${dpkgArch##*-}" in \
      "amd64" | "armhf" | "arm64") tiniArch="tini-static-$dpkgArch" ;;\
      *) echo >&2 "unsupported architecture: ${dpkgArch}"; exit 1 ;; \
   esac; \
   cd /tmp \
   && git clone https://github.com/ncopa/su-exec.git \
   && cd su-exec \
   && git checkout -q $SUEXEC_VERSION \
   && make su-exec-static \
   && cd /tmp \
   && wget -q -O tini https://github.com/krallin/tini/releases/download/$TINI_VERSION/$tiniArch \
   && chmod +x tini

# Now comes the actual target image, which aims to be as small as possible.
FROM busybox:1.31.1-glibc
LABEL maintainer="Alok Nerurkar <alok@stream.space>"

# Get the ipfs binary, entrypoint script, and TLS CAs from the build container.
ENV SRC_DIR /ss-ipfs-lite
ENV EXEC_DIR /ss-ipfs-lite/examples/litepeer/
ENV AUTO_DIR /ss-ipfs-lite/examples/auto/

COPY --from=0 $EXEC_DIR/ss-light-linux-amd64 /usr/local/bin/lite
COPY --from=0 $AUTO_DIR/auto /usr/local/bin/auto
COPY --from=0 /tmp/su-exec/su-exec-static /sbin/su-exec
COPY --from=0 /tmp/tini /sbin/tini

ENTRYPOINT ["auto"]
