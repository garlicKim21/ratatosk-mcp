# ratatosk-mcp — static Go binary on distroless. stdio by default; set
# MCP_HTTP_ADDR=:8080 for the in-cluster streamable-HTTP mode (Helm chart).

# Read-only client of the public ratatosk.io /v1 API. No credentials.
FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /ratatosk-mcp .

FROM gcr.io/distroless/static-debian12:nonroot
# MCP Registry ownership verification — must equal server.json .name
LABEL io.modelcontextprotocol.server.name="io.github.garlicKim21/ratatosk-mcp"
COPY --from=build /ratatosk-mcp /ratatosk-mcp
ENTRYPOINT ["/ratatosk-mcp"]
