package quran

import (
	"testing"

	"github.com/tamnd/any-cli/kit"
)

// These tests are offline: they exercise the URI driver's pure string functions
// and the host wiring (mint, body, resolve), which need no network. The client's
// HTTP behaviour is covered in quran_test.go.

func TestDomainInfo(t *testing.T) {
	info := Domain{}.Info()
	if info.Scheme != "quran" {
		t.Errorf("Scheme = %q, want quran", info.Scheme)
	}
	if len(info.Hosts) == 0 || info.Hosts[0] != Host {
		t.Errorf("Hosts = %v, want [%s]", info.Hosts, Host)
	}
	if info.Identity.Binary != "quran" {
		t.Errorf("Identity.Binary = %q, want quran", info.Identity.Binary)
	}
}

func TestClassify(t *testing.T) {
	cases := []struct {
		in  string
		typ string
		id  string
	}{
		{"1:1", "verse", "1:1"},
		{"2:255", "verse", "2:255"},
		{"112:4", "verse", "112:4"},
		{"1", "chapter", "1"},
		{"114", "chapter", "114"},
		{"al-fatiha", "query", "al-fatiha"},
	}
	for _, tc := range cases {
		typ, id, err := Domain{}.Classify(tc.in)
		if err != nil || typ != tc.typ || id != tc.id {
			t.Errorf("Classify(%q) = (%q, %q, %v), want (%q, %q, nil)",
				tc.in, typ, id, err, tc.typ, tc.id)
		}
	}
}

func TestClassifyEmpty(t *testing.T) {
	_, _, err := Domain{}.Classify("")
	if err == nil {
		t.Error("Classify(\"\") expected error, got nil")
	}
}

func TestLocate(t *testing.T) {
	cases := []struct {
		uriType string
		id      string
		want    string
	}{
		{"verse", "1:1", "https://quran.com/1/1"},
		{"verse", "2:255", "https://quran.com/2/255"},
		{"chapter", "1", "https://quran.com/1"},
		{"chapter", "114", "https://quran.com/114"},
	}
	for _, tc := range cases {
		got, err := Domain{}.Locate(tc.uriType, tc.id)
		if err != nil || got != tc.want {
			t.Errorf("Locate(%q, %q) = (%q, %v), want (%q, nil)",
				tc.uriType, tc.id, got, err, tc.want)
		}
	}
}

func TestLocateUnknownType(t *testing.T) {
	_, err := Domain{}.Locate("unknown", "foo")
	if err == nil {
		t.Error("Locate(unknown, foo) expected error, got nil")
	}
}

func TestIsVerseKey(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"1:1", true},
		{"2:255", true},
		{"114:6", true},
		{"1", false},
		{"", false},
		{":1", false},
		{"1:", false},
		{"a:b", false},
	}
	for _, tc := range cases {
		got := isVerseKey(tc.s)
		if got != tc.want {
			t.Errorf("isVerseKey(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestHostWiring(t *testing.T) {
	h, err := kit.Open()
	if err != nil {
		t.Fatal(err)
	}

	ch := &Chapter{ID: 1, NameSimple: "Al-Fatihah", NameArabic: "الفاتحة", VersesCount: 7, RevelationPlace: "makkah"}
	u, err := h.Mint(ch)
	if err != nil {
		t.Fatalf("Mint: %v", err)
	}
	if want := "quran://chapter/1"; u.String() != want {
		t.Errorf("Mint = %q, want %q", u.String(), want)
	}

	got, err := h.ResolveOn("quran", "1:1")
	if err != nil || got.String() != "quran://verse/1:1" {
		t.Errorf("ResolveOn = (%q, %v), want quran://verse/1:1", got.String(), err)
	}
}
