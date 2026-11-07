package channel

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	"altalune.id/oagent/internal/enduser"
	"altalune.id/oagent/internal/ingest"
	"altalune.id/oagent/internal/kernel/db"
	"altalune.id/oagent/internal/kernel/ratelimit"
	"altalune.id/oagent/internal/kernel/tenant"
	"altalune.id/oagent/internal/run"
)

const (
	badSignatureEvery = time.Minute
	senderCap         = 10
	maxDisplayName    = 64
	resultStampEvery  = time.Minute
)

var (
	digitRun         = regexp.MustCompile(`\+?\p{Nd}(?:[\p{Zs}\p{Pd}.()]*\p{Nd}){7,}`)
	errDelivered     = errors.New("channel: delivery already taken")
	errNoChannelTime = errors.New("channel: message has no channel time")
)

// IngestRule limits deliveries per connection to perMinute a minute, burst at once.
func IngestRule(perMinute, burst int) ratelimit.Rule {
	return ratelimit.Rule{Name: "ingest-connection", Rate: perMinute, Per: time.Minute, Burst: burst}
}

// EndUsers resolves and names the people who write to a connection; the enduser service implements it.
type EndUsers interface {
	Upsert(ctx context.Context, projectID uuid.UUID, ids ...enduser.Identity) (*enduser.EndUser, error)
	Lookup(ctx context.Context, projectID uuid.UUID, id enduser.Identity) (*enduser.EndUser, error)
	SetDisplayName(ctx context.Context, id uuid.UUID, name string) error
}

// Inbox stores an inbound message on its thread; the run store implements it.
type Inbox interface {
	AppendChannelMessage(ctx context.Context, in run.ChannelMessage) (run.ChannelAppend, error)
}

// IngestParams configures NewIngest; a nil Limiter limits nothing.
type IngestParams struct {
	Service  *Service
	Store    Store
	UOW      db.Transactor
	Provider Provider
	EndUsers EndUsers
	Inbox    Inbox
	Kicker   run.Kicker
	Limiter  ratelimit.Limiter
	// Rule is the per-connection limit; the zero value is 120 a minute, 30 at once.
	Rule ratelimit.Rule
	Log  *slog.Logger
}

// Ingest takes one provider's verified deliveries into the inbox of the connection's agent.
type Ingest struct{ p IngestParams }

type resolved struct {
	conn    *Connection
	secrets Secrets
}

// NewIngest builds the ingest handler for one provider.
func NewIngest(p IngestParams) *Ingest {
	if p.Limiter == nil {
		p.Limiter = ratelimit.Disabled()
	}
	if p.Rule.Name == "" {
		p.Rule = IngestRule(120, 30)
	}
	if p.Log == nil {
		p.Log = slog.Default()
	}
	return &Ingest{p: p}
}

// Binding is the provider entry the ingest surface registers.
func (g *Ingest) Binding() ingest.Provider {
	return ingest.Provider{Resolver: g, Verifier: g, Handler: g}
}

// Resolve opens an active, secured connection. SECURITY: unknown, disabled and unsecured answer alike from one lookup.
func (g *Ingest) Resolve(ctx context.Context, publicID string) (ingest.Connection, bool, error) {
	ref, err := g.p.Store.Resolve(ctx, publicID)
	if IsNotFoundError(err) || (err == nil && (ref.Status != StatusActive || !ref.SecretSet)) {
		return ingest.Connection{}, false, nil
	}
	if err != nil {
		return ingest.Connection{}, false, err
	}
	ctx = tenant.Into(ctx, tenant.Context{OrgID: ref.OrgID, ProjectID: ref.ProjectID})
	c, err := g.p.Service.Get(ctx, ref.ID)
	if err != nil {
		return ingest.Connection{}, false, err
	}
	secrets, err := g.p.Service.OpenSecrets(ctx, c)
	if err != nil {
		return ingest.Connection{}, false, err
	}
	return ingest.Connection{PublicID: c.PublicID, Secrets: secrets.Signing, Value: resolved{conn: c, secrets: secrets}}, true, nil
}

// Verify checks the delivery with the provider; a failure stamps the connection at most once a minute.
func (g *Ingest) Verify(r *http.Request, body []byte, c ingest.Connection) error {
	res, ok := c.Value.(resolved)
	if !ok {
		return errors.New("channel: unresolved connection")
	}
	err := g.p.Provider.Verify(r, body, *res.conn, res.secrets)
	if err != nil {
		if stampErr := g.p.Store.RecordBadSignature(scoped(r.Context(), res.conn), res.conn.ID, time.Now()); stampErr != nil {
			g.p.Log.WarnContext(r.Context(), "channel: record bad signature", "connection", res.conn.PublicID, "err", stampErr.Error())
		}
	}
	return err
}

// ServeHTTP stores one verified delivery and kicks its thread; a repeat answers 202 with no write.
func (g *Ingest) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c, _ := ingest.ConnectionFrom(r.Context())
	res, ok := c.Value.(resolved)
	if !ok {
		ingest.WriteError(w, &ingest.UnverifiedError{})
		return
	}
	conn := res.conn
	ctx := scoped(r.Context(), conn)
	log := g.p.Log.With("connection", conn.PublicID)
	if err := g.limit(ctx, conn); err != nil {
		ingest.WriteError(w, err)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		ingest.WriteError(w, &ingest.BadRequestError{})
		return
	}
	in, err := g.p.Provider.Parse(body)
	if IsIgnoredError(err) {
		w.WriteHeader(http.StatusOK)
		return
	}
	if err != nil {
		// SECURITY: a parse error can quote the body, so only the connection is logged.
		log.WarnContext(ctx, "channel: unreadable delivery")
		w.WriteHeader(http.StatusOK)
		return
	}
	if in.DeviceID != conn.DeviceID {
		g.record(ctx, log, conn, ResultDeviceMismatch)
		ingest.WriteError(w, &ingest.UnverifiedError{})
		return
	}
	g.record(ctx, log, conn, ResultConnected)
	status, err := g.take(ctx, log, conn, g.p.Provider.DeliveryID(r), in)
	if err != nil {
		ingest.WriteError(w, err)
		return
	}
	w.WriteHeader(status)
}

func (g *Ingest) limit(ctx context.Context, conn *Connection) error {
	rule := g.p.Rule
	res, err := g.p.Limiter.Allow(ctx, ratelimit.Key(rule, conn.ID.String()), rule)
	if err != nil {
		g.p.Log.ErrorContext(ctx, "channel: ingest limiter", "connection", conn.PublicID, "err", err.Error())
		return &ingest.UnavailableError{}
	}
	if !res.Allowed {
		return &ingest.RateLimitedError{RetryAfterSeconds: max(res.RetryAfterSeconds(), 1)}
	}
	return nil
}

func (g *Ingest) record(ctx context.Context, log *slog.Logger, conn *Connection, result string) {
	if err := g.p.Store.RecordResult(ctx, conn.ID, result, time.Now()); err != nil {
		log.WarnContext(ctx, "channel: record result", "result", result, "err", err.Error())
	}
}

// SECURITY: logs carry public ids only; never the body, a phone, a LID or a JID.
func (g *Ingest) take(ctx context.Context, log *slog.Logger, conn *Connection, deliveryID string, in Inbound) (int, error) {
	if deliveryID == "" || len(deliveryID) > 128 {
		return 0, &ingest.BadRequestError{}
	}
	ids := identities(in.Sender)
	if len(ids) == 0 {
		log.InfoContext(ctx, "channel: sender has no identity", "message", in.ExternalID)
		return http.StatusOK, nil
	}
	person, err := g.p.EndUsers.Upsert(ctx, conn.ProjectID, ids...)
	if err != nil {
		log.ErrorContext(ctx, "channel: end user", "message", in.ExternalID, "err", err.Error())
		return 0, &ingest.UnavailableError{}
	}
	log = log.With("end_user", person.PublicID, "message", in.ExternalID)
	if name := CleanDisplayName(in.Sender.DisplayName); name != "" && name != person.DisplayName {
		if err := g.p.EndUsers.SetDisplayName(ctx, person.ID, name); err != nil {
			log.WarnContext(ctx, "channel: display name", "err", err.Error())
		}
	}
	text, err := g.rewriteMentions(ctx, conn.ProjectID, in.Text, in.Mentions)
	if err != nil {
		log.ErrorContext(ctx, "channel: mentions", "err", err.Error())
		return 0, &ingest.UnavailableError{}
	}
	msg, err := inboxMessage(in, text, person.ID)
	if err != nil {
		return unstorable(ctx, log)
	}
	var landed run.ChannelAppend
	err = g.p.UOW.RunInTx(ctx, func(ctx context.Context) error {
		inserted, err := g.p.Store.InsertDelivery(ctx, conn, deliveryID, time.Now())
		if err != nil {
			return err
		}
		if !inserted {
			return errDelivered
		}
		landed, err = g.p.Inbox.AppendChannelMessage(ctx, channelMessage(conn, in, person.ID, msg))
		return err
	})
	if errors.Is(err, errDelivered) {
		return http.StatusAccepted, nil
	}
	if err != nil {
		log.ErrorContext(ctx, "channel: store delivery", "err", err.Error())
		return 0, &ingest.UnavailableError{}
	}
	log.DebugContext(ctx, "channel: delivery taken", "pending", landed.Pending, "duplicate", landed.Duplicate)
	if landed.Pending {
		if err := g.p.Kicker.Kick(ctx, run.Kick{OrgID: conn.OrgID, ThreadID: landed.ThreadID}); err != nil {
			log.WarnContext(ctx, "channel: kick", "err", err.Error())
		}
	}
	return http.StatusAccepted, nil
}

// SECURITY: an unstorable delivery is answered 200 so it is not retried, and logged without its body.
func unstorable(ctx context.Context, log *slog.Logger) (int, error) {
	log.WarnContext(ctx, "channel: unstorable delivery")
	return http.StatusOK, nil
}

func identities(s Sender) []enduser.Identity {
	var ids []enduser.Identity
	if s.Phone != "" {
		ids = append(ids, enduser.Identity{Kind: enduser.KindWAPhone, Value: s.Phone})
	}
	if s.LID != "" {
		ids = append(ids, enduser.Identity{Kind: enduser.KindWALid, Value: s.LID})
	}
	return ids
}

// SECURITY: a mention token holds a member's phone or LID digits; each becomes a known display name or @member before storage.
func (g *Ingest) rewriteMentions(ctx context.Context, projectID uuid.UUID, text string, mentions []string) (string, error) {
	for _, digits := range mentions {
		name, err := g.mentionName(ctx, projectID, digits)
		if err != nil {
			return "", err
		}
		text = regexp.MustCompile(`@`+regexp.QuoteMeta(digits)+`\b`).ReplaceAllLiteralString(text, "@"+name)
	}
	return text, nil
}

func (g *Ingest) mentionName(ctx context.Context, projectID uuid.UUID, digits string) (string, error) {
	for _, kind := range []enduser.Kind{enduser.KindWAPhone, enduser.KindWALid} {
		e, err := g.p.EndUsers.Lookup(ctx, projectID, enduser.Identity{Kind: kind, Value: digits})
		if enduser.IsNotFoundError(err) {
			continue
		}
		if err != nil {
			return "", err
		}
		if e.DisplayName != "" {
			return e.DisplayName, nil
		}
		break
	}
	return "member", nil
}

// CleanDisplayName strips control characters and brackets, masks digit runs of 8 or more, and caps the name at 64 runes.
func CleanDisplayName(raw string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), strings.ContainsRune("[](){}<>", r):
			return ' '
		}
		return r
	}, raw)
	cleaned = digitRun.ReplaceAllString(cleaned, "***")
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if utf8.RuneCountInString(cleaned) > maxDisplayName {
		cleaned = strings.TrimSpace(string([]rune(cleaned)[:maxDisplayName]))
	}
	return cleaned
}

type imageSource struct {
	Type     string `json:"type"`
	Value    string `json:"value"`
	MimeType string `json:"mimeType"`
}

type contentPart struct {
	Type   string       `json:"type"`
	Text   string       `json:"text,omitempty"`
	Source *imageSource `json:"source,omitempty"`
	ID     string       `json:"id,omitempty"`
}

// ChannelMeta is the storage-only note on a channel message: its provider type and whether it was a voice note.
type ChannelMeta struct {
	Type  string `json:"type"`
	Voice bool   `json:"voice,omitempty"`
}

func inboxMessage(in Inbound, text string, speaker uuid.UUID) (run.Message, error) {
	if in.Time.IsZero() {
		return run.Message{}, errNoChannelTime
	}
	var content any = noted(in, text)
	if in.Type == "image" && in.Media != nil {
		parts := []contentPart{}
		if text != "" {
			parts = append(parts, contentPart{Type: "text", Text: text})
		}
		parts = append(parts, contentPart{Type: "image", Source: &imageSource{Type: "data", MimeType: in.Media.Mime}, ID: in.ExternalID})
		content = parts
	}
	meta := ChannelMeta{Type: in.Type, Voice: in.Media != nil && in.Media.Voice}
	body, err := json.Marshal(map[string]any{"id": uuid.NewString(), "role": "user", "content": content, "channel": meta})
	if err != nil {
		return run.Message{}, err
	}
	at := in.Time
	return run.Message{Role: run.RoleUser, Body: body, SpeakerID: &speaker, ExternalMessageID: in.ExternalID, ChannelTime: &at}, nil
}

func noted(in Inbound, text string) string {
	note := ""
	switch {
	case in.Type == "audio" && in.Media != nil && in.Media.Voice:
		note = "[voice note]"
	case in.Type == "audio", in.Type == "video", in.Type == "document":
		note = "[" + in.Type + "]"
	case (in.Type == "location" || in.Type == "contact") && text != "":
		return "[" + in.Type + ": " + text + "]"
	case in.Type == "location" || in.Type == "contact":
		return "[" + in.Type + "]"
	}
	return strings.TrimSpace(note + " " + text)
}

func channelMessage(conn *Connection, in Inbound, person uuid.UUID, msg run.Message) run.ChannelMessage {
	out := run.ChannelMessage{
		ProjectID: conn.ProjectID, AgentID: conn.AgentID, ConnectionID: conn.ID, ChatKey: in.ChatKey, Kind: string(in.Kind),
		Message: msg, SenderCap: senderCap,
	}
	switch in.Kind {
	case KindGroup:
		out.ChannelAddress = in.ChatAddress
	default:
		out.Actor, out.ReplyVia = person, "wa_lid"
		if in.Sender.Phone != "" {
			out.ReplyVia = "wa_phone"
		}
	}
	return out
}

func scoped(ctx context.Context, c *Connection) context.Context {
	return tenant.Into(ctx, tenant.Context{OrgID: c.OrgID, ProjectID: c.ProjectID})
}
