# syntax=docker/dockerfile:1

FROM golang:1.26.7-bookworm AS build

ARG COREDNS_VERSION=v1.14.7
ARG COREDNS_COMMIT=427fc80ed9ca47f354585eb30a3f1332950856c4

RUN git -c advice.detachedHead=false clone \
    --branch "${COREDNS_VERSION}" \
    --depth 1 \
    https://github.com/coredns/coredns.git \
    /src/coredns

WORKDIR /src/coredns

RUN test "$(git rev-parse HEAD)" = "${COREDNS_COMMIT}"

COPY go.mod go.sum /src/coredns-umbrella/
COPY *.go /src/coredns-umbrella/

RUN awk '/^forward:forward$/ { print "umbrella:github.com/xdkr/coredns-umbrella" } { print }' \
      plugin.cfg > plugin.cfg.tmp \
    && mv plugin.cfg.tmp plugin.cfg \
    && go mod edit \
      -require=github.com/xdkr/coredns-umbrella@v0.0.0 \
      -replace=github.com/xdkr/coredns-umbrella=/src/coredns-umbrella \
    && go generate coredns.go \
    && go get

RUN CGO_ENABLED=0 go build \
      -trimpath \
      -tags=grpcnotrace \
      -ldflags="-s -w -X github.com/coredns/coredns/coremain.GitCommit=${COREDNS_VERSION}-umbrella-e2e" \
      -o /out/coredns . \
    && /out/coredns -plugins | grep -qx umbrella

FROM scratch

COPY --from=build /out/coredns /coredns
COPY e2e/Corefile /etc/coredns/Corefile

USER 65532:65532
EXPOSE 1053/udp 1053/tcp
ENTRYPOINT ["/coredns"]
CMD ["-conf", "/etc/coredns/Corefile"]

