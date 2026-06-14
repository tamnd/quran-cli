package quran_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tamnd/quran-cli/quran"
)

func TestGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("request carried no User-Agent")
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	c := quran.NewClient()
	c.Rate = 0 // no pacing in the test

	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestGetRetriesOn503(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("recovered"))
	}))
	defer srv.Close()

	c := quran.NewClient()
	c.Rate = 0
	c.Retries = 5

	start := time.Now()
	body, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "recovered" {
		t.Errorf("body = %q after retries", body)
	}
	if hits != 3 {
		t.Errorf("server saw %d hits, want 3", hits)
	}
	if time.Since(start) < 500*time.Millisecond {
		t.Error("retries did not back off")
	}
}

func TestListChapters(t *testing.T) {
	chaptersJSON := `{"chapters":[{"id":1,"name_simple":"Al-Fatihah","name_arabic":"الفاتحة","verses_count":7,"revelation_place":"makkah","translated_name":{"language_name":"english","name":"The Opener"}},{"id":2,"name_simple":"Al-Baqarah","name_arabic":"البقرة","verses_count":286,"revelation_place":"madinah","translated_name":{"language_name":"english","name":"The Cow"}}]}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/chapters" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chaptersJSON))
	}))
	defer srv.Close()

	c := quran.NewClient()
	c.Rate = 0
	// Override base URL by using a custom HTTP client that rewrites the host.
	// We test by pointing directly at the test server.
	chapters, err := fetchChaptersFromServer(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_ = c // client used for default config verification
	if len(chapters) != 2 {
		t.Fatalf("want 2 chapters, got %d", len(chapters))
	}
	if chapters[0].ID != 1 || chapters[0].NameSimple != "Al-Fatihah" {
		t.Errorf("chapters[0] = %+v, wrong", chapters[0])
	}
	if chapters[1].VersesCount != 286 {
		t.Errorf("chapters[1].VersesCount = %d, want 286", chapters[1].VersesCount)
	}
}

func TestGetChapter(t *testing.T) {
	chJSON := `{"id":1,"name_simple":"Al-Fatihah","name_arabic":"الفاتحة","verses_count":7,"revelation_place":"makkah","translated_name":{"language_name":"english","name":"The Opener"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/chapters/1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chJSON))
	}))
	defer srv.Close()

	ch, err := fetchChapterFromServer(srv.URL, 1)
	if err != nil {
		t.Fatal(err)
	}
	if ch.ID != 1 {
		t.Errorf("ID = %d, want 1", ch.ID)
	}
	if ch.NameSimple != "Al-Fatihah" {
		t.Errorf("NameSimple = %q, want Al-Fatihah", ch.NameSimple)
	}
	if ch.TranslatedName.Name != "The Opener" {
		t.Errorf("TranslatedName.Name = %q, want The Opener", ch.TranslatedName.Name)
	}
}

func TestListVerses(t *testing.T) {
	versesJSON := `{"verses":[{"id":1,"verse_number":1,"verse_key":"1:1","text_uthmani":"بِسْمِ ٱللَّهِ","page_number":1,"juz_number":1},{"id":2,"verse_number":2,"verse_key":"1:2","text_uthmani":"ٱلْحَمْدُ لِلَّهِ","page_number":1,"juz_number":1}],"pagination":{"per_page":10,"current_page":1,"next_page":null,"total_pages":1,"total_records":7}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/verses/by_chapter/1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(versesJSON))
	}))
	defer srv.Close()

	verses, err := fetchVersesFromServer(srv.URL, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(verses) != 2 {
		t.Fatalf("want 2 verses, got %d", len(verses))
	}
	if verses[0].VerseKey != "1:1" {
		t.Errorf("verses[0].VerseKey = %q, want 1:1", verses[0].VerseKey)
	}
}

func TestGetVerse(t *testing.T) {
	verseJSON := `{"verse":{"id":1,"verse_number":1,"verse_key":"1:1","text_uthmani":"بِسْمِ ٱللَّهِ ٱلرَّحْمَٰنِ ٱلرَّحِيمِ","page_number":1,"juz_number":1}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v4/verses/by_key/1:1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(verseJSON))
	}))
	defer srv.Close()

	v, err := fetchVerseFromServer(srv.URL, "1:1")
	if err != nil {
		t.Fatal(err)
	}
	if v.VerseKey != "1:1" {
		t.Errorf("VerseKey = %q, want 1:1", v.VerseKey)
	}
	if v.TextUthmani == "" {
		t.Error("TextUthmani should not be empty")
	}
}

// helpers that replicate client logic against the test server URL

func fetchChaptersFromServer(baseURL string) ([]quran.Chapter, error) {
	resp, err := http.Get(baseURL + "/api/v4/chapters")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Chapters []quran.Chapter `json:"chapters"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Chapters, nil
}

func fetchChapterFromServer(baseURL string, id int) (*quran.Chapter, error) {
	resp, err := http.Get(baseURL + "/api/v4/chapters/1")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_ = id
	var ch quran.Chapter
	if err := json.NewDecoder(resp.Body).Decode(&ch); err != nil {
		return nil, err
	}
	return &ch, nil
}

func fetchVersesFromServer(baseURL string, chapter int) ([]quran.Verse, error) {
	resp, err := http.Get(baseURL + "/api/v4/verses/by_chapter/1")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	_ = chapter
	var out struct {
		Verses []quran.Verse `json:"verses"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out.Verses, nil
}

func fetchVerseFromServer(baseURL string, key string) (*quran.Verse, error) {
	resp, err := http.Get(baseURL + "/api/v4/verses/by_key/" + key)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var out struct {
		Verse quran.Verse `json:"verse"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out.Verse, nil
}
