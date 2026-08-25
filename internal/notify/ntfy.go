// Package notify sends push notifications to a phone via an ntfy server.
//
// This is the one part of taskii that sends anything off the machine. It is
// off by default and does nothing without a topic, because an ntfy topic is a
// shared channel rather than an account: on the public server anyone who knows
// or guesses the topic name can subscribe to it and read every message sent
// there. Titles of work events are exactly the sort of thing that matters, so
// the topic should be long and unguessable, or the server self-hosted.
package notify

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultServer is the public ntfy instance.
const DefaultServer = "https://ntfy.sh"

// timeout bounds a push. A phone notification that arrives late is useless, and
// blocking on a dead network would be worse.
const timeout = 8 * time.Second

// ErrNotConfigured means pushing was attempted without a topic.
var ErrNotConfigured = errors.New("no ntfy topic configured")

// Config is everything needed to push.
type Config struct {
	Enabled bool
	// Server is the ntfy base URL. Empty means the public instance.
	Server string
	// Topic is the channel to publish to.
	Topic string
}

// Ready reports whether a push would be attempted.
func (c Config) Ready() bool {
	return c.Enabled && strings.TrimSpace(c.Topic) != ""
}

// URL is the endpoint a message is posted to.
func (c Config) URL() string {
	server := strings.TrimSpace(c.Server)
	if server == "" {
		server = DefaultServer
	}
	return strings.TrimRight(server, "/") + "/" + strings.TrimSpace(c.Topic)
}

// Message is one push.
type Message struct {
	Title string
	Body  string
	// Tags become emoji on the phone. "alarm_clock" and "calendar" are the
	// two this app sends.
	Tags []string
	// Priority is ntfy's 1..5; 4 is "high", which is what a reminder minutes
	// before an event warrants.
	Priority int
}

// Push publishes a message.
//
// A failure is returned rather than retried: the desktop notification has
// already been shown by the time this runs, so a lost push degrades the
// experience instead of losing the reminder.
func Push(ctx context.Context, cfg Config, msg Message) error {
	if !cfg.Ready() {
		return ErrNotConfigured
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL(), strings.NewReader(msg.Body))
	if err != nil {
		return err
	}
	if msg.Title != "" {
		req.Header.Set("Title", msg.Title)
	}
	if len(msg.Tags) > 0 {
		req.Header.Set("Tags", strings.Join(msg.Tags, ","))
	}
	if msg.Priority > 0 {
		req.Header.Set("Priority", fmt.Sprint(msg.Priority))
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Drain so the connection can be reused.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// The topic is deliberately left out of the error: it is a secret in
		// everything but name, and errors end up on screen and in logs.
		return fmt.Errorf("ntfy returned %s", resp.Status)
	}
	return nil
}
