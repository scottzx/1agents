package sources

// google_puller.go is the framework skeleton for the Google data source (People
// / Calendar / Gmail). Like the Microsoft skeleton it wires into the bronze
// pipeline so the account model and config UI work today; the real API pull
// (OAuth token + syncToken / historyId incremental) is deferred and Pull is a
// no-op empty page. Google has no 大陆 endpoint, so it is intl-only (see
// vendors.go Regions).

// Google API base endpoints (intl only).
const (
	googlePeopleBase   = "https://people.googleapis.com/v1"
	googleCalendarBase = "https://www.googleapis.com/calendar/v3"
	googleGmailBase    = "https://gmail.googleapis.com/gmail/v1"
)

// googlePuller pulls Google collections for one account.
type googlePuller struct {
	kinds []string
}

// NewGooglePuller builds a Google puller for the enabled kinds. Region is fixed
// (intl) so it takes no region argument.
func NewGooglePuller(kinds []string) Puller {
	return &googlePuller{kinds: kinds}
}

func (p *googlePuller) Source() string { return VendorGoogle }

func (p *googlePuller) Discover(accountID string) ([]Collection, error) {
	out := make([]Collection, 0, len(p.kinds))
	for _, k := range p.kinds {
		out = append(out, Collection{Kind: k, ID: k})
	}
	return out, nil
}

func (p *googlePuller) Pull(accountID string, c Collection, cur Cursor) ([]RawRecord, Cursor, bool, error) {
	// TODO: real Google incremental — People connections.list with syncToken,
	// Calendar events.list with syncToken, Gmail history.list with historyId;
	// persist the returned token as a "sync_token" cursor. Framework skeleton:
	// empty page, done.
	return nil, cur, true, nil
}
