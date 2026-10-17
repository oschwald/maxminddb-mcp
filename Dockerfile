FROM alpine:3.24

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S maxmind && \
    adduser -u 1001 -S -G maxmind maxmind

# Create directories
RUN mkdir -p /config /data && \
    chown -R maxmind:maxmind /config /data

# Copy the binary
COPY maxminddb-mcp /usr/local/bin/maxminddb-mcp

# Make sure the binary is executable
RUN chmod +x /usr/local/bin/maxminddb-mcp

# Switch to non-root user
USER maxmind

# Set environment variables
ENV MAXMINDDB_MCP_CONFIG=/config/config.toml
ENV MAXMINDDB_MCP_LOG_LEVEL=info
ENV MAXMINDDB_MCP_LOG_FORMAT=json

# Expose common config and data directories as volumes
VOLUME ["/config", "/data"]

# Set working directory
WORKDIR /data

ENTRYPOINT ["/usr/local/bin/maxminddb-mcp"]