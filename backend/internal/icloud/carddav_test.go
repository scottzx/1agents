package icloud

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestFetchContactsFlow drives the full CardDAV discovery + query flow against a
// canned iCloud-shaped server, verifying auth is sent, each step follows the
// returned hrefs, and vCards parse into contacts.
func TestFetchContactsFlow(t *testing.T) {
	const principal = `<multistatus xmlns="DAV:"><response><href>/</href><propstat><prop>
		<current-user-principal><href>/123/principal/</href></current-user-principal>
		</prop></propstat></response></multistatus>`
	const homeset = `<multistatus xmlns="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav"><response><href>/123/principal/</href><propstat><prop>
		<c:addressbook-home-set><href>/123/carddavhome/</href></c:addressbook-home-set>
		</prop></propstat></response></multistatus>`
	const collections = `<multistatus xmlns="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav">
		<response><href>/123/carddavhome/</href><propstat><prop><resourcetype><collection/></resourcetype></prop></propstat></response>
		<response><href>/123/carddavhome/card/</href><propstat><prop><resourcetype><collection/><c:addressbook/></resourcetype></prop></propstat></response>
		</multistatus>`
	report := `<multistatus xmlns="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav">
		<response><href>/123/carddavhome/card/a.vcf</href><propstat><prop><c:address-data>` +
		"BEGIN:VCARD\nFN:Wang Wu\nTEL:+8613900000000\nEND:VCARD\n" +
		`</c:address-data></prop></propstat></response>
		<response><href>/123/carddavhome/card/b.vcf</href><propstat><prop><c:address-data>` +
		"BEGIN:VCARD\nFN:Zhao Liu\nEMAIL:zhao@example.com\nEND:VCARD\n" +
		`</c:address-data></prop></propstat></response></multistatus>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusMultiStatus)
		switch {
		case r.Method == "PROPFIND" && r.URL.Path == "/":
			w.Write([]byte(principal))
		case r.Method == "PROPFIND" && r.URL.Path == "/123/principal/":
			w.Write([]byte(homeset))
		case r.Method == "PROPFIND" && r.URL.Path == "/123/carddavhome/":
			w.Write([]byte(collections))
		case r.Method == "REPORT" && r.URL.Path == "/123/carddavhome/card/":
			w.Write([]byte(report))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := NewClient("user@icloud.com", "abcd-efgh-ijkl-mnop")
	c.base = srv.URL

	contacts, err := c.FetchContacts()
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 2 {
		t.Fatalf("want 2 contacts, got %d", len(contacts))
	}
	if contacts[0].Name != "Wang Wu" || len(contacts[0].Phones) != 1 || contacts[0].Phones[0] != "+8613900000000" {
		t.Errorf("contact[0] wrong: %+v", contacts[0])
	}
	if contacts[1].Name != "Zhao Liu" || len(contacts[1].Emails) != 1 {
		t.Errorf("contact[1] wrong: %+v", contacts[1])
	}
}

func TestAuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewClient("x", "y")
	c.base = srv.URL
	_, err := c.FetchContacts()
	if err == nil || !strings.Contains(err.Error(), "authentication failed") {
		t.Errorf("want auth failure, got %v", err)
	}
}
