FROM golang:1.14.4-buster
LABEL maintainer="Sabyasachi Patra <sabyasachi@stream.space>"

# Install deps
RUN apt-get update && apt-get install -y \
   libssl-dev \
   ca-certificates

ENV SRC_DIR /ss-ipfs-lite
ENV EXEC_DIR /ss-ipfs-lite/examples/litepeer/
ENV AUTO_DIR /ss-ipfs-lite/examples/auto/

# Download packages first so they can be cached.
COPY go.mod go.sum $SRC_DIR/
RUN cd $SRC_DIR \
  && go mod download

COPY . $SRC_DIR

RUN cd $EXEC_DIR && go build -o ss-light-linux-amd64
RUN cd $AUTO_DIR && go build -o auto

# Now comes the actual target image, which aims to be as small as possible.
FROM busybox:1.31.1-glibc
LABEL maintainer="Sabyasachi Patra <sabyasachi@stream.space>"

# Get the ipfs binary, entrypoint script, and TLS CAs from the build container.
ENV SRC_DIR /ss-ipfs-lite
ENV EXEC_DIR /ss-ipfs-lite/examples/litepeer/
ENV AUTO_DIR /ss-ipfs-lite/examples/auto/

COPY --from=0 /etc/ssl/certs /etc/ssl/certs
COPY --from=0 $EXEC_DIR/ss-light-linux-amd64 /usr/local/bin/lite
COPY --from=0 $AUTO_DIR/auto /usr/local/bin/auto
COPY --from=0 $AUTO_DIR/env.json /home/env.json

ENTRYPOINT ["auto"]
