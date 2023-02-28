package mahimahi

import (
	"bufio"
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
	cmd := exec.Command(m.ReplayServerBin)
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
	fmt.Printf("%v\n", cmd.Env)
	cmd_stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error stdoutpipe: %v", err)
	}

	err = cmd.Start()
	if err != nil {
		return fmt.Errorf("error running replayshell command: %v", err)
	}

	fmt.Printf("starting read loop\n")

	cmd_stdout_buf := bufio.NewReader(cmd_stdout)
	res, err := http.ReadResponse(cmd_stdout_buf, nil)
	if err != nil {
		return fmt.Errorf("error parsing http response from replay server: %v", err)
	}

	w.WriteHeader(res.StatusCode)
	for name, values := range r.Header {
		for _, value := range values {
			w.Header().Set(name, value)
		}
	}

	buf := make([]byte, 8192)

	for {
		n_read, err := res.Body.Read(buf)
		if err != nil {
			return fmt.Errorf("error reading from body: %v", err)
		}
		fmt.Printf("Read %v from body\n", n_read)

		//fmt.Printf("read %v", buf[:n_read])
		wrote, err := w.Write(buf[:n_read])
		if err != nil {
			return fmt.Errorf("error writing to stream")
		}
		fmt.Printf("wrote %v\n", wrote)
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
