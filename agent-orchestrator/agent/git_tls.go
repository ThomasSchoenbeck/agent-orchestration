package agent

import (
	"crypto/tls"
	"net/http"

	"github.com/go-git/go-git/v5/plumbing/transport/client"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// InstallGitTLS makes go-git's HTTPS transport use the given TLS config (the
// same one the API client uses), so the agent can clone/push the orchestrator's
// repos over HTTPS with a self-signed/pinned server certificate. No-op when
// tlsCfg is nil (plain HTTP or system-root trust). Global, process-wide.
func InstallGitTLS(tlsCfg *tls.Config) {
	if tlsCfg == nil {
		return
	}
	hc := &http.Client{
		Transport: &http.Transport{TLSClientConfig: tlsCfg.Clone()},
	}
	client.InstallProtocol("https", githttp.NewClient(hc))
}
