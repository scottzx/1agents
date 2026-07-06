package govern

import (
	"fmt"
	"strings"

	"github.com/scottzx/1Agents/backend/internal/data"
	"github.com/scottzx/1Agents/backend/internal/icloud"
	"github.com/scottzx/1Agents/backend/internal/sources"
)

// silver_icloud.go — Apple/iCloud governor + vCard parser (issue #399).

func SilverIcloudContacts(src *sources.Store, dst *data.Store) (int, error) {
	return runSilver(src, dst, sources.SourceICloud, sources.KindContact, parseIcloudContact, dst.UpsertIcloudContacts)
}

func parseIcloudContact(r sources.StoredRecord) []data.SilverIcloudContact {
	cards := splitVCards(r.Payload)
	out := make([]data.SilverIcloudContact, 0, len(cards))
	for i, card := range cards {
		ext := r.UID
		if len(cards) > 1 {
			ext = fmt.Sprintf("%s#%d", r.UID, i)
		}
		c := data.SilverIcloudContact{AccountID: r.AccountID, ExternalID: ext, Deleted: r.Deleted, UpdatedAt: r.FetchedAt}
		for _, p := range icloud.VCardProps(card) {
			key, val := p[0], strings.TrimSpace(p[1])
			switch key {
			case "FN":
				c.FullName = val
			case "N":
				parts := strings.Split(val, ";")
				if len(parts) > 0 {
					c.FamilyName = parts[0]
				}
				if len(parts) > 1 {
					c.GivenName = parts[1]
				}
			case "TEL":
				if val != "" {
					c.Phones = append(c.Phones, val)
				}
			case "EMAIL":
				if val != "" {
					c.Emails = append(c.Emails, val)
				}
			case "ORG":
				c.Org = strings.SplitN(val, ";", 2)[0]
			case "TITLE":
				c.Title = val
			case "BDAY":
				c.Birthday = val
			case "NICKNAME":
				c.Nickname = val
			case "NOTE":
				c.Note = val
			case "IMPP":
				if val != "" {
					c.IMHandles = append(c.IMHandles, val)
				}
			case "URL":
				if val != "" {
					c.URLs = append(c.URLs, val)
				}
			case "ADR":
				if val != "" {
					c.Addresses = append(c.Addresses, val)
				}
			}
		}
		out = append(out, c)
	}
	return out
}

// splitVCards splits a payload into individual BEGIN:VCARD..END:VCARD blocks so
// each card is a distinct silver row (a resource is usually one card).
func splitVCards(payload string) []string {
	lines := strings.Split(strings.ReplaceAll(payload, "\r\n", "\n"), "\n")
	var cards []string
	var cur []string
	in := false
	for _, l := range lines {
		u := strings.ToUpper(strings.TrimSpace(l))
		if strings.HasPrefix(u, "BEGIN:VCARD") {
			in, cur = true, nil
		}
		if in {
			cur = append(cur, l)
		}
		if strings.HasPrefix(u, "END:VCARD") {
			in = false
			cards = append(cards, strings.Join(cur, "\n"))
		}
	}
	if len(cards) == 0 {
		return []string{payload}
	}
	return cards
}
