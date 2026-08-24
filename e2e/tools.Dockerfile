# syntax=docker/dockerfile:1

FROM golang:1.26.7-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download github.com/miekg/dns

COPY e2e/client ./e2e/client
COPY e2e/fakecisco ./e2e/fakecisco

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/client ./e2e/client \
    && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/fake-cisco ./e2e/fakecisco

FROM scratch

COPY --from=build /out/client /client
COPY --from=build /out/fake-cisco /fake-cisco

USER 65532:65532

