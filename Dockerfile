# Build the static service binary with the approved Go toolchain. The native
# HTML/CSS/JS frontend is embedded via //go:embed, so no Node build is needed.
FROM docker.m.daocloud.io/library/golang:1.26.3-bookworm

WORKDIR /app

# Go dependencies (cached layer).
COPY go.mod go.sum ./
RUN GOPROXY=https://goproxy.cn,direct GOSUMDB=sum.golang.google.cn go mod download

COPY . .
# The runtime image must be able to execute --smoke-test directly.
RUN CGO_ENABLED=0 GOTOOLCHAIN=local go build -o /out/orbitops .

FROM docker.m.daocloud.io/library/alpine:3.20

WORKDIR /app

COPY --from=0 /out/orbitops /app/orbitops

ENTRYPOINT ["/app/orbitops"]

CMD ["bash"]
