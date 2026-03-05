// Package mcp provides the Model Context Protocol (MCP) client interface.
// MCP allows agents to connect to external tool servers (databases, APIs, file systems, etc.)
// using a standardized protocol.
//
// To connect a real MCP server:
//  1. Implement the Client interface (or use the official MCP Go SDK when available)
//  2. Replace NoopClient with your implementation in the dependency injection setup
//  3. Inject the client into agents that need external tool access
//
// Reference: https://modelcontextprotocol.io
package mcp

import "context"

// Tool represents an MCP tool exposed by a server.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ToolResult is the result of an MCP tool call.
type ToolResult struct {
	Content interface{} `json:"content"`
	IsError bool        `json:"isError"`
}

// Client is the interface for communicating with an MCP server.
type Client interface {
	// ListTools returns all tools available on the connected MCP server.
	ListTools(ctx context.Context) ([]Tool, error)
	// CallTool invokes a named tool with the given arguments.
	CallTool(ctx context.Context, name string, args map[string]interface{}) (*ToolResult, error)
	// Close closes the connection to the MCP server.
	Close() error
}

// NoopClient is a placeholder that satisfies the Client interface.
// Replace this with a real implementation when MCP servers are available.
type NoopClient struct{}

func NewNoopClient() *NoopClient { return &NoopClient{} }

func (n *NoopClient) ListTools(_ context.Context) ([]Tool, error) { return nil, nil }
func (n *NoopClient) CallTool(_ context.Context, _ string, _ map[string]interface{}) (*ToolResult, error) {
	return &ToolResult{Content: "MCP not configured", IsError: true}, nil
}
func (n *NoopClient) Close() error { return nil }
