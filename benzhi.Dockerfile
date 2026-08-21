# Benzhi evaluation Dockerfile for task145-orbitops.
# Pure-Go project with go.sum; the native HTML/CSS/JS frontend is embedded via
# //go:embed so no Node build is needed. Template A (Go + go.sum, no Node).
FROM golang:1.26.3

WORKDIR /app

# Go dependencies (cached layer).
COPY go.mod go.sum ./
RUN GOPROXY=https://goproxy.cn,direct GOSUMDB=sum.golang.google.cn go mod download

COPY . .
# Pre-compile once; the compile cache stays in the image. Does not affect edits.
RUN CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...

# Container starts in a shell for evaluation.
CMD ["bash"]
