package autosign

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"
)

func TestRunnerLogsStageAndResultWithoutRawError(t *testing.T) {
	var out bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&out)
	defer log.SetOutput(previous)
	s := testStore(t)
	start := at("2026-10-15T00:59:58Z")
	save(t, s, start)
	f := &fakeSign{err: errors.New("secret-password https://example.com/?token=private")}
	NewRunner(s, f, start).Tick(context.Background(), start.Add(3*time.Second))
	text := out.String()
	for _, want := range []string{"[autosign]", "task_id=1", "stage=sign", "status=failed"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
	if strings.Contains(text, "secret-password") || strings.Contains(text, "token=") {
		t.Fatal("raw error leaked")
	}
	if f.calls != 1 {
		t.Fatal("diagnostics repeated submission")
	}
}
func TestRunnerLogsDuplicateWithoutSubmittingAgain(t *testing.T) {
	var out bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&out)
	defer log.SetOutput(previous)
	s := testStore(t)
	start := at("2026-10-15T00:59:58Z")
	save(t, s, start)
	f := &fakeSign{}
	NewRunner(s, f, start).Tick(context.Background(), start.Add(3*time.Second))
	NewRunner(s, f, start).Tick(context.Background(), start.Add(4*time.Second))
	if f.calls != 1 || !strings.Contains(out.String(), "status=not_claimed") {
		t.Fatalf("missing duplicate diagnostic: calls=%d logs=%q", f.calls, out.String())
	}
}
