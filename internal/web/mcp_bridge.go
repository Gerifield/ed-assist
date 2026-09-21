package web

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPBridge implements gemini.MCPCaller and manages communication with the MCP server
// via in-process server pointer, HTTP/SSE client, or stdio client.
type MCPBridge struct {
	mode      string // "inprocess", "http", "stdio"
	endpoint  string // e.g. "http://127.0.0.1:8080/sse" or binary path for stdio
	mcpServer *server.MCPServer
	client    *client.Client
}

func defaultInitializeRequest() mcp.InitializeRequest {
	req := mcp.InitializeRequest{}
	req.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	req.Params.ClientInfo = mcp.Implementation{
		Name:    "ed-assist-web",
		Version: "1.0.0",
	}
	return req
}

// NewInProcessBridge creates a bridge directly connecting to an in-process MCP server instance.
func NewInProcessBridge(s *server.MCPServer) (*MCPBridge, error) {
	c, err := client.NewInProcessClient(s)
	if err != nil {
		return nil, fmt.Errorf("failed creating in-process MCP client: %w", err)
	}
	ctx := context.Background()
	if _, err := c.Initialize(ctx, defaultInitializeRequest()); err != nil {
		return nil, fmt.Errorf("failed initializing in-process MCP client: %w", err)
	}

	return &MCPBridge{
		mode:      "inprocess",
		mcpServer: s,
		client:    c,
	}, nil
}

// NewHTTPBridge connects to an MCP server running over HTTP/SSE.
func NewHTTPBridge(sseURL string) (*MCPBridge, error) {
	if sseURL == "" {
		sseURL = "http://127.0.0.1:8080/sse"
	}
	if !strings.HasPrefix(sseURL, "http://") && !strings.HasPrefix(sseURL, "https://") {
		sseURL = "http://" + sseURL
	}
	if !strings.HasSuffix(sseURL, "/sse") {
		sseURL = strings.TrimRight(sseURL, "/") + "/sse"
	}

	c, err := client.NewSSEMCPClient(sseURL)
	if err != nil {
		return nil, fmt.Errorf("failed creating SSE MCP client for %s: %w", sseURL, err)
	}

	ctx := context.Background()
	if err := c.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed starting SSE client connection to %s: %w", sseURL, err)
	}

	if _, err := c.Initialize(ctx, defaultInitializeRequest()); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("failed initializing SSE MCP client: %w", err)
	}

	return &MCPBridge{
		mode:     "http",
		endpoint: sseURL,
		client:   c,
	}, nil
}

// NewStdioBridge spawns an ed-assist binary subprocess and communicates via stdio MCP.
func NewStdioBridge(binPath string, args ...string) (*MCPBridge, error) {
	if binPath == "" {
		binPath = "./ed-assist"
	}
	c, err := client.NewStdioMCPClient(binPath, nil, args...)
	if err != nil {
		return nil, fmt.Errorf("failed creating stdio MCP client with binary %s: %w", binPath, err)
	}

	ctx := context.Background()
	if _, err := c.Initialize(ctx, defaultInitializeRequest()); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("failed initializing stdio MCP client: %w", err)
	}

	return &MCPBridge{
		mode:     "stdio",
		endpoint: binPath,
		client:   c,
	}, nil
}

// ListTools retrieves the available tools from the connected MCP server.
func (b *MCPBridge) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	if b.client == nil {
		return nil, fmt.Errorf("mcp client is not connected")
	}
	res, err := b.client.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, err
	}
	return res.Tools, nil
}

// CallTool invokes a tool by name with arguments and formats text response.
func (b *MCPBridge) CallTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	if b.client == nil {
		return "", fmt.Errorf("mcp client is not connected")
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = arguments

	res, err := b.client.CallTool(ctx, req)
	if err != nil {
		return "", err
	}

	if res.IsError {
		var errSb strings.Builder
		for _, c := range res.Content {
			if textContent, ok := c.(mcp.TextContent); ok {
				errSb.WriteString(textContent.Text)
			}
		}
		return "", fmt.Errorf("tool execution returned error: %s", errSb.String())
	}

	var sb strings.Builder
	for _, c := range res.Content {
		if textContent, ok := c.(mcp.TextContent); ok {
			sb.WriteString(textContent.Text)
		}
	}
	return sb.String(), nil
}

// Mode returns the active MCP transport mode.
func (b *MCPBridge) Mode() string {
	return b.mode
}

// Close terminates client connections if applicable.
func (b *MCPBridge) Close() error {
	if b.client != nil {
		return b.client.Close()
	}
	return nil
}
