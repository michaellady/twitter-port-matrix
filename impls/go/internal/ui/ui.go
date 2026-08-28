// Package ui is the Tier-4 Phase 2 HTMX UI — mirror of the Rust primary's
// crates/server/src/ui.rs. All routes here live in the TCB. The UI is a
// thin server-rendered view: every form posts to a handler that calls
// into the verified service core, then re-renders the relevant fragment.
//
// Actor identity (per converged Tier-4 plan K20+K23):
//   - / shows a handle picker. Submitting it POSTs to /_session which
//     sets a cookie acting_handle=<handle>.<hmac>.
//   - The HMAC uses UI_COOKIE_HMAC_KEY env var. If unset, falls back to
//     a per-process random key — fine for local dev, NOT durable for prod.
//     In prod the key is set as a Fly secret (see DEPLOY.md).
//   - Every UI write reads the cookie's handle and uses it as the actor.
//   - This is NOT auth — anyone can claim any handle.
package ui

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/michaellady/twitter-port-matrix-impl-go/internal/dom"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/service"
	"github.com/michaellady/twitter-port-matrix-impl-go/internal/store"
)

const cookieName = "acting_handle"

// Mount registers the UI routes on the given mux. Must be called BEFORE
// the JSON-API routes are mounted so /_ui/follow et al. take precedence
// (Go's http.ServeMux uses longest-prefix-match, so /_ui/follow is more
// specific than /follow regardless of registration order, but explicit
// is better).
func Mount(mux *http.ServeMux, svc *service.Service) {
	h := &handlers{svc: svc}
	mux.HandleFunc("/", h.homeOrLogin)
	mux.HandleFunc("/_session", h.setSession)
	mux.HandleFunc("/_session/clear", h.clearSession)
	mux.HandleFunc("/u/", h.profile)
	mux.HandleFunc("/compose", h.compose)
	mux.HandleFunc("/register", h.register)
	mux.HandleFunc("/_ui/follow", h.uiFollow)
	mux.HandleFunc("/_ui/unfollow", h.uiUnfollow)
}

type handlers struct{ svc *service.Service }

// =============================================================================
// HMAC cookie signing
// =============================================================================

var (
	signingKeyOnce sync.Once
	signingKeyVal  []byte
)

func signingKey() []byte {
	signingKeyOnce.Do(func() {
		if hexKey := os.Getenv("UI_COOKIE_HMAC_KEY"); hexKey != "" {
			if bytes, err := hex.DecodeString(hexKey); err == nil && len(bytes) >= 16 {
				signingKeyVal = bytes
				return
			}
			fmt.Fprintln(os.Stderr, "ui: UI_COOKIE_HMAC_KEY set but invalid (need >=16 hex bytes); using ephemeral key")
		}
		// Dev fallback: per-process random. NOT durable across restarts.
		signingKeyVal = make([]byte, 32)
		_, _ = rand.Read(signingKeyVal)
	})
	return signingKeyVal
}

func sign(handle string) string {
	mac := hmac.New(sha256.New, signingKey())
	mac.Write([]byte(handle))
	tag := mac.Sum(nil)[:16]
	return handle + "." + hex.EncodeToString(tag)
}

func verifyCookie(value string) (string, bool) {
	dot := strings.LastIndex(value, ".")
	if dot < 1 {
		return "", false
	}
	handle, tag := value[:dot], value[dot+1:]
	if len(tag) != 32 {
		return "", false
	}
	expected := sign(handle)
	expectedTag := expected[strings.LastIndex(expected, ".")+1:]
	if subtle.ConstantTimeCompare([]byte(tag), []byte(expectedTag)) != 1 {
		return "", false
	}
	return handle, true
}

func currentActor(r *http.Request) (string, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return "", false
	}
	return verifyCookie(c.Value)
}

func setActorCookie(w http.ResponseWriter, handle string) {
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    sign(handle),
		Path:     "/",
		MaxAge:   30 * 24 * 60 * 60,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// =============================================================================
// Page rendering
// =============================================================================

const layoutTpl = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{.Title}} · twitter-formal-go</title>
<script src="https://unpkg.com/htmx.org@2.0.4" integrity="sha384-HGfztofotfshcF7+8n44JQL2oJmowVChPTg48S+jvZoztPfvwD79OC/LTtG6dMp+" crossorigin="anonymous"></script>
<style>
  :root { color-scheme: light dark; }
  * { box-sizing: border-box; }
  body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif;
         max-width: 640px; margin: 0 auto; padding: 1rem;
         line-height: 1.5; color: #111; background: #fafafa; }
  @media (prefers-color-scheme: dark) {
    body { color: #eee; background: #111; }
    .tweet, .compose, .panel { background: #1a1a1a; border-color: #333; }
    a { color: #6cb6ff; }
  }
  header { display: flex; justify-content: space-between; align-items: baseline;
           border-bottom: 1px solid #ccc; padding-bottom: .5rem; margin-bottom: 1rem; }
  header h1 { margin: 0; font-size: 1.2rem; }
  header h1 a { color: inherit; text-decoration: none; }
  .actor { font-size: .85rem; color: #666; }
  .verified-badge { display: inline-block; background: #00add8; color: white;
                    padding: 1px 6px; border-radius: 3px; font-size: .7rem;
                    vertical-align: middle; }
  .tweet { border: 1px solid #ddd; border-radius: 6px; padding: .75rem 1rem;
           margin-bottom: .5rem; background: white; }
  .tweet-meta { font-size: .8rem; color: #666; margin-bottom: .25rem; }
  .tweet-meta a { color: inherit; text-decoration: none; }
  .tweet-meta a:hover { text-decoration: underline; }
  .compose, .panel { border: 1px solid #ddd; border-radius: 6px; padding: 1rem;
                     margin-bottom: 1rem; background: white; }
  .compose textarea, .panel input[type=text] { width: 100%; padding: .5rem;
    border: 1px solid #ccc; border-radius: 4px; font: inherit; }
  .compose textarea { min-height: 4em; resize: vertical; }
  button { padding: .4rem .9rem; border: 1px solid #00add8; background: #00add8;
           color: white; border-radius: 4px; cursor: pointer; font: inherit; }
  button:hover { background: #008bb0; }
  button.link { background: none; color: #00add8; border: none; padding: 0;
                text-decoration: underline; cursor: pointer; }
  button.secondary { background: white; color: #00add8; }
  .error { color: #b00; padding: .5rem; border: 1px solid #f5b5b5;
           border-radius: 4px; background: #fff5f5; margin-bottom: 1rem; }
  .empty { color: #888; text-align: center; padding: 2rem 0; }
  footer { font-size: .8rem; color: #888; margin-top: 2rem; padding-top: 1rem;
           border-top: 1px solid #ccc; }
  footer a { color: inherit; }
</style>
</head>
<body>
<header>
  <h1><a href="/">twitter-formal-go <span class="verified-badge">verified</span></a></h1>
  {{if .Actor}}
    <span class="actor">acting as <strong>@{{.Actor}}</strong> ·
      <form method="post" action="/_session/clear" style="display:inline">
        <button class="link">switch</button>
      </form></span>
  {{else}}
    <span class="actor"><a href="/">log in</a></span>
  {{end}}
</header>
{{.Body}}
<footer>
  Verified backend (Gobra + TLC). UI is in TCB.
  <a href="/version">/version</a> · <a href="/healthz">/healthz</a> ·
  <a href="https://github.com/michaellady/twitter-port-matrix-impl-go">source</a>
</footer>
</body>
</html>`

var pageTemplate = template.Must(template.New("layout").Parse(layoutTpl))

type pageData struct {
	Title string
	Actor string
	Body  template.HTML
}

func renderPage(w http.ResponseWriter, title string, actor string, body string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTemplate.Execute(w, pageData{Title: title, Actor: actor, Body: template.HTML(body)})
}

func loginBody(prefill, errMsg string) string {
	errorHTML := ""
	if errMsg != "" {
		errorHTML = fmt.Sprintf(`<div class="error">%s</div>`, template.HTMLEscapeString(errMsg))
	}
	return fmt.Sprintf(`
<h2>Pick a handle</h2>
<p>This is a verified-backend demo. There is no auth — pick any handle and you become it.</p>
%s
<div class="panel">
  <form method="post" action="/_session">
    <input type="text" name="handle" placeholder="alice" value="%s" autofocus>
    <p>Try <code>alice</code>, <code>bob</code>, or <code>carol</code> — they're seeded.
       Or pick something new and click "create handle" below.</p>
    <button type="submit">log in as this handle</button>
    <button type="submit" formaction="/register" class="secondary">create handle &amp; log in</button>
  </form>
</div>`, errorHTML, template.HTMLEscapeString(prefill))
}

func renderTweets(tweets []dom.Tweet) string {
	if len(tweets) == 0 {
		return `<p class="empty">no tweets yet</p>`
	}
	var b strings.Builder
	for _, t := range tweets {
		fmt.Fprintf(&b, `<div class="tweet"><div class="tweet-meta"><a href="/u/%s">@%s</a> · t=%d · id=%d</div><div>%s</div></div>`,
			template.HTMLEscapeString(t.Author),
			template.HTMLEscapeString(t.Author),
			t.CreatedAt,
			t.ID,
			template.HTMLEscapeString(t.Text),
		)
	}
	return b.String()
}

// =============================================================================
// Handlers
// =============================================================================

func (h *handlers) homeOrLogin(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	actor, ok := currentActor(r)
	if !ok {
		renderPage(w, "log in", "", loginBody("", ""))
		return
	}
	if !h.svc.HasUser(actor) {
		// Cookie names a handle that doesn't exist (e.g. demo data was
		// reset). Surface an honest message; let them re-register.
		renderPage(w, "log in", "", loginBody(actor,
			fmt.Sprintf("the handle @%s is no longer registered (state may have been reset). Re-register or pick another.", actor)))
		return
	}
	tweets, _ := h.svc.HomeTimeline(actor, 50, 0)
	body := fmt.Sprintf(`
<h2>home timeline</h2>
<div class="compose">
  <form method="post" action="/compose">
    <textarea name="text" placeholder="say something verified&hellip;" required></textarea>
    <button type="submit">post</button>
  </form>
</div>
%s`, renderTweets(tweets))
	renderPage(w, "home", actor, body)
}

func (h *handlers) profile(w http.ResponseWriter, r *http.Request) {
	handle := strings.TrimPrefix(r.URL.Path, "/u/")
	if handle == "" || strings.Contains(handle, "/") {
		http.NotFound(w, r)
		return
	}
	if !h.svc.HasUser(handle) {
		renderPage(w, "not found", "", `<div class="error">no such handle</div>`)
		return
	}
	actor, _ := currentActor(r)
	tweets, _ := h.svc.HomeTimeline(handle, 50, 0) // shows their own + their follows; close enough for the demo
	// Filter to just this handle's tweets for the profile view
	own := tweets[:0]
	for _, t := range tweets {
		if t.Author == handle {
			own = append(own, t)
		}
	}
	followBtn := ""
	if actor != "" && actor != handle {
		followBtn = fmt.Sprintf(`
<div class="panel">
  <form method="post" action="/_ui/follow" style="display:inline">
    <input type="hidden" name="to" value="%s">
    <button type="submit">follow</button>
  </form>
  <form method="post" action="/_ui/unfollow" style="display:inline">
    <input type="hidden" name="to" value="%s">
    <button type="submit" class="secondary">unfollow</button>
  </form>
</div>`, template.HTMLEscapeString(handle), template.HTMLEscapeString(handle))
	}
	body := fmt.Sprintf(`<h2>@%s's tweets</h2>%s%s`,
		template.HTMLEscapeString(handle), followBtn, renderTweets(own))
	renderPage(w, "@"+handle, actor, body)
}

func (h *handlers) setSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	handle := strings.TrimSpace(r.PostFormValue("handle"))
	if handle == "" {
		renderPage(w, "log in", "", loginBody("", "handle is empty"))
		return
	}
	if !h.svc.HasUser(handle) {
		renderPage(w, "log in", "", loginBody(handle,
			fmt.Sprintf("@%s isn't registered. Use \"create handle\" instead.", handle)))
		return
	}
	setActorCookie(w, handle)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handlers) clearSession(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handlers) register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	handle := strings.TrimSpace(r.PostFormValue("handle"))
	if handle == "" {
		renderPage(w, "log in", "", loginBody("", "handle is empty"))
		return
	}
	_, err := h.svc.CreateUser(handle)
	if err != nil && !errors.Is(err, store.ErrHandleTaken) {
		renderPage(w, "log in", "", loginBody(handle,
			fmt.Sprintf("could not register @%s: %v", handle, err)))
		return
	}
	setActorCookie(w, handle)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handlers) compose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	actor, ok := currentActor(r)
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(r.PostFormValue("text"))
	if text == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	_, _ = h.svc.PostTweet(actor, text)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *handlers) uiFollow(w http.ResponseWriter, r *http.Request) {
	h.uiFollowOrUnfollow(w, r, true)
}

func (h *handlers) uiUnfollow(w http.ResponseWriter, r *http.Request) {
	h.uiFollowOrUnfollow(w, r, false)
}

func (h *handlers) uiFollowOrUnfollow(w http.ResponseWriter, r *http.Request, follow bool) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	actor, ok := currentActor(r)
	if !ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	to := strings.TrimSpace(r.PostFormValue("to"))
	if to == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if follow {
		_ = h.svc.Follow(actor, to)
	} else {
		_ = h.svc.Unfollow(actor, to)
	}
	http.Redirect(w, r, "/u/"+to, http.StatusSeeOther)
}
