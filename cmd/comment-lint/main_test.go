package main

import (
	"bytes"
	"strings"
	"testing"
)

const testdata = "../../internal/tool/commentlint/testdata/"

func TestRun(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{name: "compliant file", args: []string{testdata + "compliant.go"}, code: 0, stdout: "no violations"},
		{name: "violation fails", args: []string{testdata + "section_header.go"}, code: 1, stdout: "[section-header]", stderr: "check failed"},
		{name: "list never fails", args: []string{"-list", testdata + "section_header.go"}, code: 0, stdout: "[section-header]"},
		{name: "unknown flag", args: []string{"-nope"}, code: 2, stderr: "flag provided but not defined"},
		{name: "missing root", args: []string{testdata + "missing"}, code: 2, stderr: "comment-lint:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if got := run(&stdout, &stderr, tt.args); got != tt.code {
				t.Fatalf("run() = %d, want %d\nstdout: %s\nstderr: %s", got, tt.code, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), tt.stdout) {
				t.Errorf("stdout = %q, want it to contain %q", stdout.String(), tt.stdout)
			}
			if !strings.Contains(stderr.String(), tt.stderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.stderr)
			}
		})
	}
}
