# Builder
FROM --platform=$BUILDPLATFORM whatwewant/builder-go:v1.20-1 as builder

WORKDIR /build

COPY go.mod ./

COPY go.sum ./

RUN go mod download

COPY . .

ARG TARGETOS

ARG TARGETARCH

RUN CGO_ENABLED=0 \
  GOOS=${TARGETOS} \
  GOARCH=${TARGETARCH} \
  go build \
  -trimpath \
  -ldflags '-w -s -buildid=' \
  -v -o agent ./cmd/agent

# Server
FROM whatwewant/alpine:v3.17-1

LABEL MAINTAINER="Zero<tobewhatwewant@gmail.com>"

LABEL org.opencontainers.image.source="https://github.com/go-idp/agent"

ARG VERSION=latest

RUN zmicro update -a

RUN zmicro plugin install eunomia

ENV MODE=production

COPY --from=builder /build/agent /bin

RUN agent --version

EXPOSE 8838

CMD agent server
