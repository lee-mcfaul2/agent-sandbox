# Stage 1: build
FROM golang:1.25-alpine AS build
WORKDIR /src
RUN apk add --no-cache cosign skopeo ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .

# In production: pull lib-agent-prompt OCI bundle and cosign keyless-verify.
# For local builds, BUNDLE_SOURCE=fixture copies the in-repo fixtures into
# internal/schemas/embedded/ so the image can be built and run without an OCI registry.
ARG BUNDLE_SOURCE=fixture
ARG LIB_AGENT_PROMPT_REF=""
ARG COSIGN_IDENTITY=""
ARG COSIGN_OIDC_ISSUER="https://token.actions.githubusercontent.com"
RUN set -eux; \
    if [ "$BUNDLE_SOURCE" = "oci" ]; then \
        test -n "$LIB_AGENT_PROMPT_REF"; \
        test -n "$COSIGN_IDENTITY"; \
        cosign verify \
            --certificate-identity "$COSIGN_IDENTITY" \
            --certificate-oidc-issuer "$COSIGN_OIDC_ISSUER" \
            "$LIB_AGENT_PROMPT_REF"; \
        skopeo copy "docker://$LIB_AGENT_PROMPT_REF" "dir:/tmp/bundle"; \
        mkdir -p internal/schemas/embedded/lib-agent-prompt; \
        cp -r /tmp/bundle/* internal/schemas/embedded/lib-agent-prompt/; \
    else \
        mkdir -p internal/schemas/embedded/lib-agent-prompt; \
        cp -r internal/schemas/testdata/lib-agent-prompt/* internal/schemas/embedded/lib-agent-prompt/; \
    fi

RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/sandbox ./cmd/sandbox

# Stage 2: distroless runtime
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/sandbox /sandbox
USER nonroot
ENTRYPOINT ["/sandbox"]
