package quran

import (
	"context"
	"fmt"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/any-cli/kit/errs"
)

// domain.go exposes quran as a kit Domain: a driver that a multi-domain
// host (ant) enables with a single blank import,
//
//	import _ "github.com/tamnd/quran-cli/quran"
//
// exactly as a database/sql program enables a driver with `import _
// "github.com/lib/pq"`. The init below registers it; the host then dereferences
// quran:// URIs by routing to the operations Register installs. The same
// Domain also builds the standalone quran binary (see cli.NewApp), so the
// binary and a host share one source of truth.
func init() { kit.Register(Domain{}) }

// Domain is the quran driver. It carries no state; the per-run client is
// built by the factory Register hands kit.
type Domain struct{}

// Info describes the scheme, the hostnames a pasted link is matched against, and
// the identity reused for the binary's help and version.
func (Domain) Info() kit.DomainInfo {
	return kit.DomainInfo{
		Scheme: "quran",
		Hosts:  []string{Host},
		Identity: kit.Identity{
			Binary: "quran",
			Short:  "A command line for the Quran API.",
			Long: `A command line for the Quran API.

quran reads public Quran data from api.quran.com over plain HTTPS, shapes it
into clean records, and prints output that pipes into the rest of your tools.
No API key, nothing to run alongside it.`,
			Site: "quran.com",
			Repo: "https://github.com/tamnd/quran-cli",
		},
	}
}

// Register installs the client factory and every operation onto app.
func (Domain) Register(app *kit.App) {
	app.SetClient(newClient)

	kit.Handle(app, kit.OpMeta{Name: "chapters", Group: "read", List: true,
		Summary: "List all 114 chapters of the Quran", URIType: "chapter"},
		listChapters)

	kit.Handle(app, kit.OpMeta{Name: "chapter", Group: "read", Single: true,
		Summary: "Fetch a chapter by number (1-114)", URIType: "chapter", Resolver: true,
		Args: []kit.Arg{{Name: "id", Help: "chapter number (1-114)"}}},
		getChapter)

	kit.Handle(app, kit.OpMeta{Name: "verses", Group: "read", List: true,
		Summary: "List verses in a chapter", URIType: "verse",
		Args: []kit.Arg{{Name: "chapter", Help: "chapter number (1-114)"}}},
		listVerses)

	kit.Handle(app, kit.OpMeta{Name: "verse", Group: "read", Single: true,
		Summary: "Fetch a single verse by key (e.g. '1:1' or '2:255')", URIType: "verse", Resolver: true,
		Args: []kit.Arg{{Name: "key", Help: "verse key (e.g. '1:1' or '2:255')"}}},
		getVerse)
}

// newClient builds the client from the host-resolved config, so a host and the
// standalone binary pace and identify themselves the same way.
func newClient(_ context.Context, cfg kit.Config) (any, error) {
	c := NewClient()
	if cfg.UserAgent != "" {
		c.UserAgent = cfg.UserAgent
	}
	if cfg.Rate > 0 {
		c.Rate = cfg.Rate
	}
	if cfg.Retries > 0 {
		c.Retries = cfg.Retries
	}
	if cfg.Timeout > 0 {
		c.HTTP.Timeout = cfg.Timeout
	}
	return c, nil
}

// --- inputs ---

type chaptersInput struct {
	Client *Client `kit:"inject"`
}

type chapterInput struct {
	ID     int     `kit:"arg" help:"chapter number (1-114)"`
	Client *Client `kit:"inject"`
}

type versesInput struct {
	Chapter int     `kit:"arg" help:"chapter number (1-114)"`
	Limit   int     `kit:"flag,inherit" help:"max verses" default:"10"`
	Client  *Client `kit:"inject"`
}

type verseInput struct {
	Key    string  `kit:"arg" help:"verse key (e.g. '1:1' or '2:255')"`
	Client *Client `kit:"inject"`
}

// --- handlers ---

func listChapters(ctx context.Context, in chaptersInput, emit func(*Chapter) error) error {
	chapters, err := in.Client.ListChapters(ctx)
	if err != nil {
		return mapErr(err)
	}
	for i := range chapters {
		if err := emit(&chapters[i]); err != nil {
			return err
		}
	}
	return nil
}

func getChapter(ctx context.Context, in chapterInput, emit func(*Chapter) error) error {
	ch, err := in.Client.GetChapter(ctx, in.ID)
	if err != nil {
		return mapErr(err)
	}
	return emit(ch)
}

func listVerses(ctx context.Context, in versesInput, emit func(*Verse) error) error {
	limit := in.Limit
	if limit <= 0 {
		limit = 10
	}
	verses, err := in.Client.ListVerses(ctx, in.Chapter, limit)
	if err != nil {
		return mapErr(err)
	}
	for i := range verses {
		if err := emit(&verses[i]); err != nil {
			return err
		}
	}
	return nil
}

func getVerse(ctx context.Context, in verseInput, emit func(*Verse) error) error {
	v, err := in.Client.GetVerse(ctx, in.Key)
	if err != nil {
		return mapErr(err)
	}
	return emit(v)
}

// --- Resolver: the URI-native string functions, pure and network-free ---

// Classify turns any accepted input into the canonical (type, id).
// "N:N" format (e.g. "1:1", "2:255") → ("verse", input)
// Pure numeric → ("chapter", input)
// Otherwise → ("query", input)
func (Domain) Classify(input string) (uriType, id string, err error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", errs.Usage("unrecognized quran reference: %q", input)
	}
	// verse key: digits:digits
	if isVerseKey(input) {
		return "verse", input, nil
	}
	// bare chapter number
	if isNumeric(input) {
		return "chapter", input, nil
	}
	return "query", input, nil
}

// Locate is the inverse: the live https URL for a (type, id).
func (Domain) Locate(uriType, id string) (string, error) {
	switch uriType {
	case "verse":
		parts := strings.SplitN(id, ":", 2)
		if len(parts) != 2 {
			return "", errs.Usage("invalid verse key %q, expected chapter:verse", id)
		}
		return fmt.Sprintf("https://quran.com/%s/%s", parts[0], parts[1]), nil
	case "chapter":
		return fmt.Sprintf("https://quran.com/%s", id), nil
	default:
		return "", errs.Usage("quran has no resource type %q", uriType)
	}
}

// --- helpers ---

func isVerseKey(s string) bool {
	idx := strings.Index(s, ":")
	if idx < 1 || idx == len(s)-1 {
		return false
	}
	return isNumeric(s[:idx]) && isNumeric(s[idx+1:])
}

func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// mapErr converts a library error into the kit error kind that carries the right
// exit code, so a host renders the same outcomes the standalone binary does.
func mapErr(err error) error {
	return err
}
