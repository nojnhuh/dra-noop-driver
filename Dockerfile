FROM golang:1.23 AS build

WORKDIR /build
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 go build -ldflags "-extldflags '-static'" -o dra-noop-kubeletplugin .

FROM scratch
COPY --from=build /build/dra-noop-kubeletplugin /usr/bin/dra-noop-kubeletplugin
ENTRYPOINT ["/usr/bin/dra-noop-kubeletplugin"]
