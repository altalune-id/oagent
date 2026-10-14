// Command protoc-gen-mcp generates MCP tool bindings for every RPC annotated with mcp.v1.tool.
package main

import (
	"google.golang.org/protobuf/compiler/protogen"

	"altalune.id/oagent/internal/tool/protocmcp"
)

func main() {
	var opts protocmcp.Options
	protogen.Options{ParamFunc: opts.Set}.Run(func(p *protogen.Plugin) error {
		return protocmcp.Generate(p, opts)
	})
}
