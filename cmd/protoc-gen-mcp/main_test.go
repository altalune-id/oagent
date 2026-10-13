package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/pluginpb"

	agentv1 "altalune.id/oagent/gen/go/agent/v1"
	authv1 "altalune.id/oagent/gen/go/auth/v1"
	invitev1 "altalune.id/oagent/gen/go/invite/v1"
	memberv1 "altalune.id/oagent/gen/go/member/v1"
	orgv1 "altalune.id/oagent/gen/go/org/v1"
	projectv1 "altalune.id/oagent/gen/go/project/v1"
	webhookv1 "altalune.id/oagent/gen/go/webhook/v1"
)

const committedParameter = "paths=source_relative,ui_prefix=ui://oagent"

func committedFiles() []protoreflect.FileDescriptor {
	return []protoreflect.FileDescriptor{
		agentv1.File_agent_v1_agent_proto,
		authv1.File_auth_v1_auth_proto,
		invitev1.File_invite_v1_invite_proto,
		memberv1.File_member_v1_member_proto,
		orgv1.File_org_v1_org_proto,
		projectv1.File_project_v1_project_proto,
		webhookv1.File_webhook_v1_webhook_proto,
	}
}

func request(parameter string, generate ...protoreflect.FileDescriptor) *pluginpb.CodeGeneratorRequest {
	req := &pluginpb.CodeGeneratorRequest{Parameter: new(parameter)}
	seen := map[string]bool{}
	var add func(fd protoreflect.FileDescriptor)
	add = func(fd protoreflect.FileDescriptor) {
		if seen[fd.Path()] {
			return
		}
		seen[fd.Path()] = true
		imports := fd.Imports()
		for i := range imports.Len() {
			add(imports.Get(i).FileDescriptor)
		}
		req.ProtoFile = append(req.ProtoFile, protodesc.ToFileDescriptorProto(fd))
	}
	for _, fd := range generate {
		add(fd)
		req.FileToGenerate = append(req.FileToGenerate, fd.Path())
	}
	return req
}

func runPlugin(t *testing.T, req *pluginpb.CodeGeneratorRequest) *pluginpb.CodeGeneratorResponse {
	t.Helper()
	raw, err := proto.Marshal(req)
	require.NoError(t, err)

	dir := t.TempDir()
	in := filepath.Join(dir, "request.binpb")
	require.NoError(t, os.WriteFile(in, raw, 0o600))
	stdin, err := os.Open(in)
	require.NoError(t, err)
	t.Cleanup(func() { _ = stdin.Close() })
	stdout, err := os.Create(filepath.Join(dir, "response.binpb"))
	require.NoError(t, err)

	oldArgs, oldIn, oldOut := os.Args, os.Stdin, os.Stdout
	t.Cleanup(func() { os.Args, os.Stdin, os.Stdout = oldArgs, oldIn, oldOut })
	os.Args, os.Stdin, os.Stdout = []string{"protoc-gen-mcp"}, stdin, stdout
	main()
	os.Args, os.Stdin, os.Stdout = oldArgs, oldIn, oldOut
	require.NoError(t, stdout.Close())

	out, err := os.ReadFile(stdout.Name())
	require.NoError(t, err)
	resp := &pluginpb.CodeGeneratorResponse{}
	require.NoError(t, proto.Unmarshal(out, resp))
	return resp
}

func TestGenerated_MatchesProtos(t *testing.T) {
	resp := runPlugin(t, request(committedParameter, committedFiles()...))
	require.Empty(t, resp.GetError())

	var names []string
	for _, f := range resp.GetFile() {
		names = append(names, f.GetName())
		committed, err := os.ReadFile(filepath.Join("../../gen/go", f.GetName()))
		require.NoError(t, err)
		assert.Equal(t, string(committed), f.GetContent(), "gen/go/%s is stale; run make generate", f.GetName())
	}

	committed, err := filepath.Glob("../../gen/go/*/v1/*mcp/*.mcp.go")
	require.NoError(t, err)
	var want []string
	for _, path := range committed {
		rel, err := filepath.Rel("../../gen/go", path)
		require.NoError(t, err)
		want = append(want, filepath.ToSlash(rel))
	}
	assert.ElementsMatch(t, want, names, "every committed MCP binding is generated, and nothing else")
}

func TestMain_SkipsFilesWithoutTools(t *testing.T) {
	resp := runPlugin(t, request(committedParameter, (&descriptorpb.FileDescriptorProto{}).ProtoReflect().Descriptor().ParentFile()))
	require.Empty(t, resp.GetError())
	assert.Empty(t, resp.GetFile())
}

func TestMain_ReportsGeneratorErrors(t *testing.T) {
	resp := runPlugin(t, request("paths=source_relative", agentv1.File_agent_v1_agent_proto))
	assert.Contains(t, resp.GetError(), "ui_prefix")
	assert.Empty(t, resp.GetFile())
}

func TestMain_ExitsOnBadParameters(t *testing.T) {
	if path := os.Getenv("PROTOC_GEN_MCP_REQUEST"); path != "" {
		stdin, err := os.Open(path) //nolint:gosec // G304: the parent test names a file in its temp dir.
		require.NoError(t, err)
		os.Args, os.Stdin = []string{"protoc-gen-mcp"}, stdin
		main()
		return
	}

	raw, err := proto.Marshal(request("paths=source_relative,bogus=1", agentv1.File_agent_v1_agent_proto))
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "request.binpb")
	require.NoError(t, os.WriteFile(path, raw, 0o600))

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestMain_ExitsOnBadParameters$")
	cmd.Env = append(os.Environ(), "PROTOC_GEN_MCP_REQUEST="+path)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	err = cmd.Run()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	assert.Equal(t, 1, exitErr.ExitCode())
	assert.Contains(t, stderr.String(), `unknown parameter "bogus"`)
}
