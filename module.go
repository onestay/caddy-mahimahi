package mahimahi

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	caddy.RegisterModule(MahiMahi{})
	httpcaddyfile.RegisterHandlerDirective("mahimahi", parseCaddyfile)
}

type MahiMahi struct {
	WorkingDir      string `json:"working_dir,omitempty"`
	RecordingDir    string `json:"recording_dir,omitempty"`
	ReplayServerBin string `json:"replay_server_bin,omitempty"`
	w               io.Writer
}

func (MahiMahi) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.mahimahi",
		New: func() caddy.Module { return new(MahiMahi) },
	}
}

func (m MahiMahi) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	// w.Write([]byte(fmt.Sprintf("recording: %s, working: %s, replayshell: %s", m.RecordingDir, m.WorkingDir, m.ReplayServerBin)))
	m.w.Write([]byte(fmt.Sprintf("Got a request %v\n", r.RemoteAddr)))
	cmd := exec.Command("/bin/sh", "-c", m.ReplayServerBin)
	cmd.Env = append(cmd.Env,
		"MAHIMAHI_CHDIR="+m.WorkingDir,
		"MAHIMAHI_RECORD_PATH="+m.RecordingDir,
		"REQUEST_METHOD="+r.Method,
		"REQUEST_URI="+r.RequestURI,
		"SERVER_PROTOCOL="+r.Proto,
		"HTTP_HOST="+r.Host,
	)

	userAgent := r.UserAgent()
	if len(userAgent) != 0 {
		cmd.Env = append(cmd.Env, "HTTP_USER_AGENT="+userAgent)
	}

	// TODO: set this based on is HTTPS is actually used
	cmd.Env = append(cmd.Env, "HTTPS=1")
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("error running replayshell command: %v", err)
	}
	w.Write([]byte(fmt.Sprintf("%v\n", cmd.Env)))
	buf := make([]byte, 8192)

	for {
		n_read, err := cmd.Stdin.Read(buf)
		if err != nil {
			return fmt.Errorf("error reading from cmd: %v", err)
		}
		w.Write(buf[:n_read])
		if n_read != 8192 {
			break
		}
	}
	return nil
}

func (m *MahiMahi) Provision(ctx caddy.Context) error {
	if len(m.ReplayServerBin) == 0 {
		m.ReplayServerBin = "/usr/local/bin/mm-replayserver"
	}

	if m.w == nil {
		m.w = os.Stdout
	}

	return nil
}

func (m *MahiMahi) Validate() error {
	if m.w == nil {
		return fmt.Errorf("no writer")
	}

	if len(m.WorkingDir) == 0 {
		return fmt.Errorf("no workingdir set")
	}

	if len(m.RecordingDir) == 0 {
		return fmt.Errorf("no recordingdir set")
	}

	return nil
}

func (m *MahiMahi) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		args := d.RemainingArgs()
		if len(args) != 0 {
			return d.ArgErr()
		}

		for d.NextBlock(0) {
			switch d.Val() {
			case "working":
				if !d.NextArg() {
					return d.ArgErr()
				}

				p := d.Val()
				p = path.Clean(p)
				if !path.IsAbs(p) {
					return d.Errf("%s must be an absolute path", p)
				}

				m.WorkingDir = p
			case "recording":
				if !d.NextArg() {
					return d.ArgErr()
				}

				p := d.Val()
				p = path.Clean(p)
				if path.IsAbs(p) {
					return d.Errf("%s can't be an absolute path", p)
				}

				m.RecordingDir = p
			case "replayserver":
				if !d.Args(&m.ReplayServerBin) {
					return d.ArgErr()
				}
			}

		}
	}

	return nil
}

func parseCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	var m MahiMahi
	err := m.UnmarshalCaddyfile(h.Dispenser)
	return m, err
}

var (
	_ caddyhttp.MiddlewareHandler = (*MahiMahi)(nil)
	_ caddy.Provisioner           = (*MahiMahi)(nil)
)
