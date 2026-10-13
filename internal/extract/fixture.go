package extract

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"regexp"

	"github.com/GhandyP/conversation-task-extractor/internal/domain"
)

// Fixture is a deterministic Extractor that reads recorded provider payloads
// from a file system: <conversationId>.json inside the fixture directory.
//
// It exists so the example runs end to end, and its tests pass, with no API key
// and no network access.
type Fixture struct {
	fsys      fs.FS
	directory string
}

// NewFixture reads payloads from dir, which may be relative to the process
// working directory.
func NewFixture(dir string) *Fixture {
	return &Fixture{fsys: os.DirFS(dir), directory: dir}
}

// NewFixtureFS reads payloads from any file system, which keeps the provider
// testable without touching disk.
func NewFixtureFS(fsys fs.FS) *Fixture {
	return &Fixture{fsys: fsys}
}

// Name identifies the provider in output and errors.
func (f *Fixture) Name() string { return "fixture" }

// Extract returns the recorded candidates for one conversation.
func (f *Fixture) Extract(ctx context.Context, conv domain.Conversation) ([]domain.TaskCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	name, err := fixtureFileName(conv.ConversationID)
	if err != nil {
		return nil, err
	}

	payload, err := fs.ReadFile(f.fsys, name)
	if err != nil {
		return nil, fmt.Errorf("fixture: no recorded payload for conversation %q (%s%s): %w",
			conv.ConversationID, f.directory, name, err)
	}
	return Decode(conv, payload)
}

// fixtureNamePattern accepts the ids used for conversation files. Anything with
// a path separator, a leading dot, or an empty id is refused, so a
// conversation id from untrusted input can never escape the fixture directory.
var fixtureNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

func fixtureFileName(conversationID string) (string, error) {
	if !fixtureNamePattern.MatchString(conversationID) {
		return "", fmt.Errorf("fixture: conversation id %q is not a safe file name", conversationID)
	}
	return conversationID + ".json", nil
}
