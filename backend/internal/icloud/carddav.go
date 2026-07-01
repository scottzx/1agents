// Package icloud pulls the user's iCloud data over Apple's standard DAV
// protocols using their Apple ID + an app-specific password — a delegated,
// user-authorized path (no reverse engineering). v1 covers Contacts via CardDAV;
// CalDAV (calendars) and IMAP (mail) are natural follow-ups behind the same
// credential. The credential lives only on the local machine (Keychain) and the
// pull runs locally, so data never transits our servers.
package icloud

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// rootURL is iCloud's CardDAV discovery entry point; auth redirects it to the
// account's partition host (pNN-contacts.icloud.com).
const rootURL = "https://contacts.icloud.com"

// Client is an authenticated iCloud CardDAV client.
type Client struct {
	http     *http.Client
	base     string
	appleID  string
	password string
}

// NewClient builds a client for the given Apple ID + app-specific password.
func NewClient(appleID, password string) *Client {
	c := &Client{base: rootURL, appleID: appleID, password: password}
	c.http = &http.Client{
		Timeout: 60 * time.Second,
		// Don't auto-follow: Go downgrades PROPFIND/REPORT → GET and drops the body
		// on 301/302, which iCloud rejects. do() follows redirects manually,
		// preserving method + body + Depth + auth across iCloud's partition-host bounce.
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return c
}

// FetchContacts discovers the default address book and returns all its contacts.
// China-region Apple IDs authenticate against the global endpoint but their data
// lives on icloud.com.cn (云上贵州); the global endpoint then hands back partition
// hosts that don't resolve. So on a DNS failure we retry discovery from the .cn
// root, which returns .cn partition hosts.
func (c *Client) FetchContacts() ([]Contact, error) {
	contacts, err := c.fetchFrom(c.base)
	if err != nil && isDNSFailure(err) && !strings.Contains(c.base, "icloud.com.cn") {
		cnBase := strings.Replace(c.base, "icloud.com", "icloud.com.cn", 1)
		return c.fetchFrom(cnBase)
	}
	return contacts, err
}

func (c *Client) fetchFrom(base string) ([]Contact, error) {
	principal, err := c.findPrincipal(base)
	if err != nil {
		return nil, err
	}
	home, err := c.findHomeSet(principal)
	if err != nil {
		return nil, err
	}
	book, err := c.findAddressbook(home)
	if err != nil {
		return nil, err
	}
	payload, err := c.queryVCards(book)
	if err != nil {
		return nil, err
	}
	return parseVCards(payload), nil
}

// isDNSFailure reports whether err is a hostname-resolution failure — the signal
// that a discovered iCloud partition host is on the wrong (non-.cn) domain.
func isDNSFailure(err error) bool {
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	return strings.Contains(err.Error(), "no such host")
}

// ── DAV plumbing ──

// multistatus mirrors a WebDAV multistatus body. Struct tags match on local name
// (namespace-agnostic), which is what we need across DAV: / carddav namespaces.
type multistatus struct {
	XMLName   xml.Name      `xml:"multistatus"`
	Responses []davResponse `xml:"response"`
}

type davResponse struct {
	Href     string `xml:"href"`
	Propstat []struct {
		Prop struct {
			CurrentUserPrincipal struct {
				Href string `xml:"href"`
			} `xml:"current-user-principal"`
			AddressbookHomeSet struct {
				Href string `xml:"href"`
			} `xml:"addressbook-home-set"`
			ResourceType struct {
				Addressbook *struct{} `xml:"addressbook"`
			} `xml:"resourcetype"`
			AddressData string `xml:"address-data"`
		} `xml:"prop"`
	} `xml:"propstat"`
}

// do issues a DAV request with Basic auth, following redirects manually so the
// method + body + Depth + auth survive iCloud's partition-host bounce. Returns the
// parsed multistatus plus the final URL (for relative-href resolution).
func (c *Client) do(method, rawURL, depth, body string) (*multistatus, *url.URL, error) {
	current := rawURL
	for redirects := 0; redirects < 10; redirects++ {
		req, err := http.NewRequest(method, current, strings.NewReader(body))
		if err != nil {
			return nil, nil, err
		}
		req.SetBasicAuth(c.appleID, c.password)
		req.Header.Set("Content-Type", "application/xml; charset=utf-8")
		if depth != "" {
			req.Header.Set("Depth", depth)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, nil, err
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			resp.Body.Close()
			if loc == "" {
				return nil, nil, fmt.Errorf("icloud: %d redirect with no Location from %s", resp.StatusCode, current)
			}
			cur, _ := url.Parse(current)
			ref, perr := url.Parse(loc)
			if perr != nil {
				return nil, nil, perr
			}
			current = cur.ResolveReference(ref).String()
			continue
		}
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, nil, fmt.Errorf("icloud: authentication failed — check the Apple ID and app-specific password")
		}
		if resp.StatusCode >= 400 {
			return nil, nil, fmt.Errorf("icloud: %s %s → %d: %s", method, current, resp.StatusCode, snippet(data))
		}
		var ms multistatus
		if err := xml.Unmarshal(data, &ms); err != nil {
			return nil, nil, fmt.Errorf("icloud: parse %s response: %w", method, err)
		}
		final, _ := url.Parse(current)
		return &ms, final, nil
	}
	return nil, nil, fmt.Errorf("icloud: too many redirects starting from %s", rawURL)
}

func (c *Client) findPrincipal(entry string) (string, error) {
	const body = `<d:propfind xmlns:d="DAV:"><d:prop><d:current-user-principal/></d:prop></d:propfind>`
	ms, base, err := c.do("PROPFIND", entry, "0", body)
	if err != nil {
		return "", err
	}
	for _, r := range ms.Responses {
		for _, ps := range r.Propstat {
			if h := ps.Prop.CurrentUserPrincipal.Href; h != "" {
				return resolve(base, h), nil
			}
		}
	}
	return "", fmt.Errorf("icloud: no current-user-principal in response")
}

func (c *Client) findHomeSet(principal string) (string, error) {
	const body = `<d:propfind xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav"><d:prop><c:addressbook-home-set/></d:prop></d:propfind>`
	ms, base, err := c.do("PROPFIND", principal, "0", body)
	if err != nil {
		return "", err
	}
	for _, r := range ms.Responses {
		for _, ps := range r.Propstat {
			if h := ps.Prop.AddressbookHomeSet.Href; h != "" {
				return resolve(base, h), nil
			}
		}
	}
	return "", fmt.Errorf("icloud: no addressbook-home-set in response")
}

func (c *Client) findAddressbook(home string) (string, error) {
	const body = `<d:propfind xmlns:d="DAV:"><d:prop><d:resourcetype/></d:prop></d:propfind>`
	ms, base, err := c.do("PROPFIND", home, "1", body)
	if err != nil {
		return "", err
	}
	for _, r := range ms.Responses {
		for _, ps := range r.Propstat {
			if ps.Prop.ResourceType.Addressbook != nil {
				return resolve(base, r.Href), nil
			}
		}
	}
	return "", fmt.Errorf("icloud: no addressbook collection found under home set")
}

func (c *Client) queryVCards(book string) (string, error) {
	const body = `<c:addressbook-query xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:carddav"><d:prop><c:address-data/></d:prop><c:filter/></c:addressbook-query>`
	ms, _, err := c.do("REPORT", book, "1", body)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, r := range ms.Responses {
		for _, ps := range r.Propstat {
			if d := ps.Prop.AddressData; d != "" {
				sb.WriteString(d)
				sb.WriteString("\n")
			}
		}
	}
	return sb.String(), nil
}

// resolve turns a possibly-relative href into an absolute URL against base.
func resolve(base *url.URL, href string) string {
	ref, err := url.Parse(href)
	if err != nil {
		return href
	}
	return base.ResolveReference(ref).String()
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200]
	}
	return s
}
