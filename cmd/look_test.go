// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildLookCmd(t *testing.T) {
	tests := []struct {
		name     string
		plain    bool
		full     bool
		wantArgs string
	}{
		{
			name:     "default flags",
			plain:    false,
			full:     false,
			wantArgs: "capture-pane -pe -t scion",
		},
		{
			name:     "plain only",
			plain:    true,
			full:     false,
			wantArgs: "capture-pane -p -t scion",
		},
		{
			name:     "full only",
			plain:    false,
			full:     true,
			wantArgs: "capture-pane -peS - -t scion",
		},
		{
			name:     "plain and full",
			plain:    true,
			full:     true,
			wantArgs: "capture-pane -pS - -t scion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := buildLookCmd(tt.plain, tt.full)

			assert.Equal(t, "/bin/sh", cmd[0])
			assert.Equal(t, "-c", cmd[1])
			assert.True(t, strings.Contains(cmd[2], tt.wantArgs),
				"expected shell command to contain %q, got %q", tt.wantArgs, cmd[2])
			assert.True(t, strings.Contains(cmd[2], `find /tmp -name "default" -type s`),
				"expected shell command to contain tmux socket discovery")
		})
	}
}

func TestPrintLookOutput_NonInteractive(t *testing.T) {
	// Save and restore global state.
	origNonInteractive := nonInteractive
	defer func() { nonInteractive = origNonInteractive }()
	nonInteractive = true

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printLookOutput("hello world\n")

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	got := string(buf[:n])

	// Non-interactive: output should be printed verbatim with no border.
	assert.Equal(t, "hello world\n", got)
	assert.NotContains(t, got, "⌄")
	assert.NotContains(t, got, "^")
}

func TestPrintLookOutput_FallbackWhenNotTerminal(t *testing.T) {
	// In test environment, stdout is a pipe so term.GetSize will fail,
	// which exercises the fallback (no border) path.
	origNonInteractive := nonInteractive
	defer func() { nonInteractive = origNonInteractive }()
	nonInteractive = false

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	printLookOutput("some output\n")

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	got := string(buf[:n])

	// Fallback: no border because stdout is not a terminal.
	assert.Equal(t, "some output\n", got)
	assert.NotContains(t, got, "⌄")
	assert.NotContains(t, got, "^")
}
