package notify

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPushSendsTitleTagsAndBody(t *testing.T) {
	var gotPath, gotTitle, gotTags, gotPriority, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotTitle = r.Header.Get("Title")
		gotTags = r.Header.Get("Tags")
		gotPriority = r.Header.Get("Priority")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
	}))
	defer srv.Close()

	cfg := Config{Enabled: true, Server: srv.URL, Topic: "my-topic"}
	err := Push(context.Background(), cfg, Message{
		Title: "taskii", Body: "standup in 5 minutes",
		Tags: []string{"alarm_clock"}, Priority: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/my-topic" {
		t.Errorf("path = %q", gotPath)
	}
	if gotTitle != "taskii" || gotBody != "standup in 5 minutes" {
		t.Errorf("title = %q body = %q", gotTitle, gotBody)
	}
	if gotTags != "alarm_clock" || gotPriority != "4" {
		t.Errorf("tags = %q priority = %q", gotTags, gotPriority)
	}
}

// Nothing may leave the machine until it is switched on and given a topic.
func TestPushRefusesWhenNotConfigured(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer srv.Close()

	for _, cfg := range []Config{
		{Enabled: false, Server: srv.URL, Topic: "t"},
		{Enabled: true, Server: srv.URL, Topic: ""},
		{Enabled: true, Server: srv.URL, Topic: "   "},
	} {
		if err := Push(context.Background(), cfg, Message{Body: "x"}); err != ErrNotConfigured {
			t.Errorf("cfg %+v gave %v, want ErrNotConfigured", cfg, err)
		}
	}
	if called {
		t.Error("a request was sent despite not being configured")
	}
}

func TestURLDefaultsToPublicServer(t *testing.T) {
	if got := (Config{Topic: "abc"}).URL(); got != DefaultServer+"/abc" {
		t.Errorf("URL = %q", got)
	}
	if got := (Config{Server: "https://push.example.com/", Topic: "abc"}).URL(); got != "https://push.example.com/abc" {
		t.Errorf("URL = %q, want the trailing slash collapsed", got)
	}
}

// The topic is a secret in all but name, so it must not turn up in an error
// that will be shown on screen.
func TestErrorDoesNotLeakTheTopic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()

	err := Push(context.Background(), Config{Enabled: true, Server: srv.URL, Topic: "secret-topic-name"}, Message{Body: "x"})
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "secret-topic-name") {
		t.Errorf("error leaks the topic: %v", err)
	}
}
