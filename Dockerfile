# Reames Agent — single-binary Docker image
# Build: docker build -t reames-agent .
# Run:   docker run -p 8787:8787 -v ~/.reames-agent:/root/.reames-agent -e DEEPSEEK_API_KEY=xxx reames-agent

FROM golang:1.25-alpine AS builder
WORKDIR /src
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=sum.golang.google.cn
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /reames-agent ./cmd/reames-agent

FROM gcr.io/distroless/static:latest
COPY --from=builder /reames-agent /reames-agent
EXPOSE 8787
ENV REAMES_AGENT_HOME=/root/.reames-agent
HEALTHCHECK --interval=30s --timeout=5s --retries=3 CMD ["/reames-agent", "serve", "--health-check"]
ENTRYPOINT ["/reames-agent", "serve", "--addr", "0.0.0.0:8787"]
