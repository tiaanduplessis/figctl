# Build a static figctl and ship it on distroless as a non-root user.
#
#   docker build -t figctl .
#   docker run --rm -e FIGMA_TOKEN figctl file tree KEY
FROM golang:1.27-alpine AS build

ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w \
      -X github.com/tiaanduplessis/figctl/internal/cli.version=${VERSION} \
      -X github.com/tiaanduplessis/figctl/internal/cli.commit=${COMMIT} \
      -X github.com/tiaanduplessis/figctl/internal/cli.date=${DATE}" \
    -o /out/figctl ./cmd/figctl

FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="figctl" \
      org.opencontainers.image.description="Agent-first Figma CLI" \
      org.opencontainers.image.source="https://github.com/tiaanduplessis/figctl" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /out/figctl /usr/local/bin/figctl

# distroless nonroot is uid 65532 with /home/nonroot as its home, the one
# writable directory in the image and where the config and cache land. Mount
# your project over it, or somewhere else and pass --out.
USER nonroot:nonroot
WORKDIR /home/nonroot

ENV FIGCTL_CREDENTIAL_STORE=file

ENTRYPOINT ["/usr/local/bin/figctl"]
CMD ["--help"]
