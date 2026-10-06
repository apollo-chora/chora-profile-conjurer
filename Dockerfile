# syntax=docker/dockerfile:1.6
#
# chora-profile-conjurer Dockerfile — standalone Go ADK agent crew
# (profile_conjurer; single conjurer sub-agent, P1 Single pattern).
#
# Build context = this repository. Shared Chora modules (chora-adk-common,
# chora-contracts) are resolved through the Go module proxy, not a workspace.

ARG GO_VERSION=1.26.6
ARG ALPINE_VERSION=3.23
ARG SERVICE_NAME=chora-profile-conjurer
ARG GIT_SHA=unknown
ARG BUILD_TIME=unknown

############################
# Stage 1 — build
############################
FROM golang:${GO_VERSION}-alpine${ALPINE_VERSION} AS builder

ARG SERVICE_NAME
ARG GIT_SHA
ARG BUILD_TIME
ARG TARGETARCH

WORKDIR /src

RUN apk add --no-cache ca-certificates git

COPY . .

RUN go mod download

ENV CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=${TARGETARCH}
RUN go build -trimpath \
      -ldflags "-s -w \
        -X main.serviceName=${SERVICE_NAME} \
        -X main.gitSHA=${GIT_SHA} \
        -X main.buildTime=${BUILD_TIME}" \
      -o /out/profile_conjurer \
      ./cmd/profile_conjurer

############################
# Stage 2 — runtime
############################
FROM gcr.io/distroless/static-debian12:nonroot

ARG SERVICE_NAME
ARG GIT_SHA
ARG BUILD_TIME

LABEL org.opencontainers.image.title="${SERVICE_NAME}" \
      org.opencontainers.image.source="https://github.com/apollo-chora/chora-profile-conjurer" \
      org.opencontainers.image.revision="${GIT_SHA}" \
      org.opencontainers.image.created="${BUILD_TIME}" \
      org.opencontainers.image.vendor="Chora Platform" \
      org.opencontainers.image.licenses="UNLICENSED" \
      io.chora.service="${SERVICE_NAME}" \
      io.chora.git-sha="${GIT_SHA}" \
      io.chora.build-time="${BUILD_TIME}"

WORKDIR /app

COPY --from=builder /out/profile_conjurer /app/profile_conjurer

USER 65532:65532
ENTRYPOINT ["/app/profile_conjurer"]
CMD ["web", "-port", "8080", "agentengine"]
