FROM golang:1.22-bullseye as builder

WORKDIR /go/src/github.com/tonkeeper/claim-api-go/

COPY go.mod .
COPY go.sum .

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd cmd
COPY pkg pkg
COPY Makefile .

# Build
RUN make build

FROM ubuntu:20.04 as runner
RUN apt-get update && \
    apt-get install -y openssl ca-certificates wget && \
    rm -rf /var/lib/apt/lists/*

RUN mkdir -p /app/lib
RUN wget -O /app/lib/libemulator.so https://github.com/ton-blockchain/ton/releases/download/v2024.08/libemulator-linux-x86_64.so
ENV LD_LIBRARY_PATH=/app/lib/
COPY --from=builder /go/src/github.com/tonkeeper/claim-api-go/bin/claim-api .

ENTRYPOINT ["/claim-api"]