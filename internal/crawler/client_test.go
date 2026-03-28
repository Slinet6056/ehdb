package crawler

import (
	"errors"
	"net/http"
	"net/url"
	"testing"
)

func TestClientValidateResponseRejectsBlankExHentaiPageWithMysteryIgneous(t *testing.T) {
	t.Helper()

	requestURL, err := url.Parse("https://exhentai.org/")
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}

	client := &Client{
		host:    "exhentai.org",
		cookies: parseCookieHeader("ipb_member_id=1; ipb_pass_hash=hash; igneous=old"),
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/html; charset=UTF-8"},
			"Set-Cookie":   []string{"igneous=mystery; Path=/; Domain=.exhentai.org"},
		},
		Request: &http.Request{URL: requestURL},
	}

	client.updateCookies(resp)
	err = client.validateResponse(resp, []byte{})
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}

	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("expected ErrAuthRequired, got %v", err)
	}

	igneous, ok := client.cookieValue("igneous")
	if !ok || igneous != "mystery" {
		t.Fatalf("expected igneous to update to mystery, got %q", igneous)
	}
}

func TestClientValidateResponseAllowsExHentaiAPIJSONResponse(t *testing.T) {
	t.Helper()

	requestURL, err := url.Parse("https://api.e-hentai.org/api.php")
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}

	client := &Client{
		host:    "exhentai.org",
		cookies: parseCookieHeader("ipb_member_id=1; ipb_pass_hash=hash; igneous=abc"),
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"application/json; charset=UTF-8"},
		},
		Request: &http.Request{URL: requestURL},
	}

	err = client.validateResponse(resp, []byte(`{"gmetadata":[]}`))
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestClientUpdateCookiesRefreshesIgneous(t *testing.T) {
	t.Helper()

	client := &Client{
		host:    "exhentai.org",
		cookies: parseCookieHeader("ipb_member_id=1; ipb_pass_hash=hash; igneous=old"),
	}

	resp := &http.Response{
		Header: http.Header{
			"Set-Cookie": []string{"igneous=fresh; Path=/; Domain=.exhentai.org"},
		},
	}

	client.updateCookies(resp)

	igneous, ok := client.cookieValue("igneous")
	if !ok {
		t.Fatal("expected igneous cookie to exist")
	}

	if igneous != "fresh" {
		t.Fatalf("expected igneous to refresh to fresh, got %q", igneous)
	}
}

func TestClientValidateResponseRejectsExistingMysteryIgneousOnBlankHTML(t *testing.T) {
	t.Helper()

	requestURL, err := url.Parse("https://exhentai.org/")
	if err != nil {
		t.Fatalf("parse request url: %v", err)
	}

	client := &Client{
		host:    "exhentai.org",
		cookies: parseCookieHeader("ipb_member_id=1; ipb_pass_hash=hash; igneous=mystery"),
	}

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type": []string{"text/html; charset=UTF-8"},
		},
		Request: &http.Request{URL: requestURL},
	}

	err = client.validateResponse(resp, []byte("   \n\t"))
	if err == nil {
		t.Fatal("expected auth error, got nil")
	}

	if !errors.Is(err, ErrAuthRequired) {
		t.Fatalf("expected ErrAuthRequired, got %v", err)
	}
}
