package gateway

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/yetone/magpie/internal/claudetui"
	"github.com/yetone/magpie/internal/testenv"
)

// tuiScriptHarness is a script harness whose runs are the interactive
// mode's, through a stand-in helper that runs the Claude Code it is handed.
func tuiScriptHarness(t *testing.T, script string) *resumeHarness {
	t.Helper()
	h := newScriptHarness(t, "ok", script)
	claudeTUIRuns = true
	t.Cleanup(func() { claudeTUIRuns = false })
	helper := filepath.Join(t.TempDir(), "magpie")
	testenv.Program(t, helper, `#!/bin/sh
[ "$1" = "`+claudetui.Command+`" ] || exit 9
shift; claude=$1; shift
exec "$claude" "$@"
`)
	claudeTUIHelper = helper
	t.Cleanup(func() { claudeTUIHelper = "" })
	return h
}

// The interactive mode's Claude Code reads CLAUDE_CODE_EXTRA_BODY only as
// it starts (a real 2.1.316: an apply_flag_settings or a --settings file
// rewritten later changed nothing it sent), so a run whose turn carries
// safeguards is given them in its environment, and a turn whose
// safeguards differ starts a new run told the whole conversation; one
// with the same goes on in the run. They were refused with the
// interactive mode on (e5f66f8a6).
func TestClaudeTUISafeguardsInEnv(t *testing.T) {
	h := tuiScriptHarness(t, `echo "extra ${CLAUDE_CODE_EXTRA_BODY-unset} betas ${ANTHROPIC_BETAS-unset}" >> $log`)

	h.extra = safeguards
	h.ask("rules", "a1")
	h.ask("rules", "a1", "a2")
	runs := h.runs()
	if len(runs) != 1 {
		t.Fatalf("the same safeguards started %d runs, want 1", len(runs))
	}
	for _, log := range runs {
		if !strings.Contains(log, `extra {"safeguards":[{"type":"dangerous_tool_use","classifier_context":"The user allows file edits in the project."}]} betas dangerous-tool-use-`) {
			t.Fatalf("the run's environment: %s", log)
		}
		if strings.Contains(log, "apply_flag_settings") {
			t.Fatalf("an interactive run was told settings it doesn't read: %s", log)
		}
	}
	h.extra = strings.Replace(safeguards, "file edits", "nothing", 1)
	h.ask("rules", "a1", "a2", "a3")
	runs = h.runs()
	if len(runs) != 2 {
		t.Fatalf("changed safeguards went on in %d runs, want a second", len(runs))
	}
	seen := false
	for _, log := range runs {
		if strings.Contains(log, `"classifier_context":"The user allows nothing in the project."`) {
			seen = true
			if !strings.Contains(log, "a1") || !strings.Contains(log, "a3") {
				t.Fatalf("the new run wasn't told the whole conversation: %s", log)
			}
		}
	}
	if !seen {
		t.Fatalf("no run has the changed safeguards: %v", runs)
	}
}

// A client's own cache marks are taken with the interactive mode on: its
// words are pasted without them (claudetui's userPieces), so Claude Code
// keeps its own prompt caching, not turned off for marks it never sends,
// and the reply says the client's marks were dropped. With caching off and
// the marks lost, every turn was written to the cache again at full price.
func TestClaudeTUIClientCacheKeepsItsOwn(t *testing.T) {
	h := tuiScriptHarness(t, `echo "cache-disabled ${DISABLE_PROMPT_CACHING-unset} extra ${CLAUDE_CODE_EXTRA_BODY-unset}" >> $log`)
	h.from = "127.0.0.1:51000"
	system := `"system":[{"type":"text","text":"shared","cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"text","text":"more"}],`
	tool := `"tools":[{"name":"read","input_schema":{"type":"object"},"cache_control":{"type":"ephemeral","ttl":"1h"}}],`
	tail := `"cache_control":{"type":"ephemeral","ttl":"1h"},`
	first := `{"role":"user","content":[{"type":"text","text":"hello","cache_control":{"type":"ephemeral","ttl":"1h"}},{"type":"text","text":"there"}]}`
	w := cacheAsk(t, h, system+tool+tail, `[`+first+`]`, "client")
	if got := w.Header().Get("X-Magpie-Cache-Marks-Dropped"); got != "tools:1,system:1,messages:2" {
		t.Fatalf("dropped %q", got)
	}
	for _, log := range h.runs() {
		if !strings.Contains(log, "cache-disabled unset extra unset") {
			t.Fatalf("an interactive run's caching: %s", log)
		}
	}
	// the next turn goes on in the run
	w = cacheAsk(t, h, system+tool+tail, `[`+first+`,{"role":"assistant","content":"ok"},{"role":"user","content":"again"}]`, "client")
	if got := w.Header().Get("X-Magpie-Cache-Marks-Dropped"); got != "tools:1,system:1,messages:2" {
		t.Fatalf("next turn dropped %q", got)
	}
	if n := len(h.runs()); n != 1 {
		t.Fatalf("the next turn started %d runs", n)
	}
}
