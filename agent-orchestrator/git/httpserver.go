package git

import (
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/format/pktline"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gogitserver "github.com/go-git/go-git/v5/plumbing/transport/server"
)

// RepoResolver maps a project slug to its bare-repo filesystem path.
type RepoResolver func(slug string) (repoPath string, err error)

// PostReceiveHook is called after a successful receive-pack with each pushed ref.
// branchName is e.g. "task/abc123". newSHA is the new HEAD hash.
type PostReceiveHook func(branchName, newSHA string)

// HTTPHandler is a net/http.Handler serving the git smart-HTTP protocol.
// Mount it under a path prefix such as "/git/".
type HTTPHandler struct {
	resolver    RepoResolver
	postReceive PostReceiveHook // may be nil
	srv         transport.Transport
}

// NewHTTPHandler creates an HTTPHandler. postReceive may be nil.
func NewHTTPHandler(resolver RepoResolver, postReceive PostReceiveHook) *HTTPHandler {
	ldr := &fsLoader{resolver: resolver}
	return &HTTPHandler{
		resolver:    resolver,
		postReceive: postReceive,
		srv:         gogitserver.NewServer(ldr),
	}
}

// urlRe matches: /git/{slug}.git/{suffix}
var urlRe = regexp.MustCompile(`^/git/([^/]+?)(?:\.git)?(/.*)?$`)

func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m := urlRe.FindStringSubmatch(r.URL.Path)
	if m == nil {
		http.NotFound(w, r)
		return
	}
	slug := m[1]
	suffix := strings.TrimPrefix(m[2], "/")

	service := r.URL.Query().Get("service")

	switch {
	case r.Method == http.MethodGet && suffix == "info/refs" && service == "git-upload-pack":
		h.serveUploadPackInfoRefs(w, r, slug)
	case r.Method == http.MethodPost && suffix == "git-upload-pack":
		h.serveUploadPack(w, r, slug)
	case r.Method == http.MethodGet && suffix == "info/refs" && service == "git-receive-pack":
		h.serveReceivePackInfoRefs(w, r, slug)
	case r.Method == http.MethodPost && suffix == "git-receive-pack":
		h.serveReceivePack(w, r, slug)
	default:
		http.NotFound(w, r)
	}
}

// --- upload-pack (clone / fetch) ---

func (h *HTTPHandler) serveUploadPackInfoRefs(w http.ResponseWriter, r *http.Request, slug string) {
	ep := endpointFor(slug)
	sess, err := h.srv.NewUploadPackSession(ep, nil)
	if err != nil {
		httpGitError(w, slug, err)
		return
	}
	defer sess.Close()

	ar, err := sess.AdvertisedReferencesContext(r.Context())
	if err != nil {
		httpGitError(w, slug, err)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	// Write the service preamble required by the smart-HTTP protocol.
	enc := pktline.NewEncoder(w)
	_ = enc.Encodef("# service=git-upload-pack\n")
	_ = enc.Flush()

	_ = ar.Encode(w)
}

func (h *HTTPHandler) serveUploadPack(w http.ResponseWriter, r *http.Request, slug string) {
	ep := endpointFor(slug)
	sess, err := h.srv.NewUploadPackSession(ep, nil)
	if err != nil {
		httpGitError(w, slug, err)
		return
	}
	defer sess.Close()

	// Stateless smart-HTTP: each POST creates a fresh session. go-git's server
	// requires AdvertisedReferences to be called first to initialise capabilities
	// before UploadPack can be invoked. We call it here and discard the result.
	if _, err := sess.AdvertisedReferencesContext(r.Context()); err != nil {
		httpGitError(w, slug, err)
		return
	}

	req := packp.NewUploadPackRequest()
	if err := req.Decode(r.Body); err != nil {
		http.Error(w, fmt.Sprintf("decode upload-pack request: %v", err), http.StatusBadRequest)
		return
	}

	resp, err := sess.UploadPack(r.Context(), req)
	if err != nil {
		httpGitError(w, slug, err)
		return
	}
	defer resp.Close()

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_ = resp.Encode(w)
}

// --- receive-pack (push) ---

func (h *HTTPHandler) serveReceivePackInfoRefs(w http.ResponseWriter, r *http.Request, slug string) {
	ep := endpointFor(slug)
	sess, err := h.srv.NewReceivePackSession(ep, nil)
	if err != nil {
		httpGitError(w, slug, err)
		return
	}
	defer sess.Close()

	ar, err := sess.AdvertisedReferencesContext(r.Context())
	if err != nil {
		httpGitError(w, slug, err)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-receive-pack-advertisement")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	enc := pktline.NewEncoder(w)
	_ = enc.Encodef("# service=git-receive-pack\n")
	_ = enc.Flush()

	_ = ar.Encode(w)
}

func (h *HTTPHandler) serveReceivePack(w http.ResponseWriter, r *http.Request, slug string) {
	ep := endpointFor(slug)
	sess, err := h.srv.NewReceivePackSession(ep, nil)
	if err != nil {
		httpGitError(w, slug, err)
		return
	}
	defer sess.Close()

	// Stateless smart-HTTP: initialise session capabilities before ReceivePack.
	if _, err := sess.AdvertisedReferencesContext(r.Context()); err != nil {
		httpGitError(w, slug, err)
		return
	}

	req := packp.NewReferenceUpdateRequest()
	if err := req.Decode(r.Body); err != nil {
		http.Error(w, fmt.Sprintf("decode receive-pack request: %v", err), http.StatusBadRequest)
		return
	}

	status, err := sess.ReceivePack(r.Context(), req)
	if err != nil {
		httpGitError(w, slug, err)
		return
	}

	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	_ = status.Encode(w)

	// Fire post-receive hook for each accepted command.
	if h.postReceive != nil {
		for _, cmd := range req.Commands {
			if cmd.New.IsZero() {
				continue // deletion — skip
			}
			branch := strings.TrimPrefix(cmd.Name.String(), "refs/heads/")
			h.postReceive(branch, cmd.New.String())
		}
	}
}

// --- helpers ---

// endpointFor builds a minimal transport.Endpoint for the given slug.
// The slug must appear in the URL path (not the host) so that ep.Path is set
// and fsLoader.Load can extract it correctly.
func endpointFor(slug string) *transport.Endpoint {
	ep, _ := transport.NewEndpoint(fmt.Sprintf("git://localhost/%s.git", slug))
	return ep
}

// httpGitError writes an appropriate HTTP error for a git transport error.
// slug is included in the server-side log to aid debugging.
func httpGitError(w http.ResponseWriter, slug string, err error) {
	if err == transport.ErrRepositoryNotFound {
		http.Error(w, "repository not found", http.StatusNotFound)
		return
	}
	log.Printf("git HTTP error slug=%q: %v", slug, err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// --- fsLoader implements gogitserver.Loader ---

type fsLoader struct {
	resolver RepoResolver
}

func (l *fsLoader) Load(ep *transport.Endpoint) (storer.Storer, error) {
	// ep.Path is like "/slug.git" — extract the slug.
	slug := strings.TrimSuffix(strings.TrimPrefix(ep.Path, "/"), ".git")
	if slug == "" {
		return nil, transport.ErrRepositoryNotFound
	}
	repoPath, err := l.resolver(slug)
	if err != nil {
		return nil, transport.ErrRepositoryNotFound
	}
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, transport.ErrRepositoryNotFound
	}
	return repo.Storer, nil
}
