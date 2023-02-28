package mahimahi

import (
	"bufio"
	"fmt"
	"net/http"
	"os/exec"
	"path"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
)

func init() {
	caddy.RegisterModule(MahiMahiTransport{})
}

type MahiMahiTransport struct {
	WorkingDir      string `json:"working_dir,omitempty"`
	RecordingDir    string `json:"recording_dir,omitempty"`
	ReplayServerBin string `json:"replay_server_bin,omitempty"`
}

func (MahiMahiTransport) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.reverse_proxy.transport.mmtransport",
		New: func() caddy.Module { return new(MahiMahiTransport) },
	}
}

func (mm MahiMahiTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	fmt.Printf("got a request to forward: %v\n", r.RemoteAddr)
	cmd := exec.Command(mm.ReplayServerBin)
	cmd.Env = append(cmd.Env,
		"MAHIMAHI_CHDIR="+mm.WorkingDir,
		"MAHIMAHI_RECORD_PATH="+mm.RecordingDir,
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
		return nil, fmt.Errorf("error stdoutpipe: %v", err)
	}

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("error running replayshell command: %v", err)
	}

	fmt.Printf("starting read loop\n")

	cmd_stdout_buf := bufio.NewReader(cmd_stdout)
	res, err := http.ReadResponse(cmd_stdout_buf, nil)
	if err != nil {
		return nil, fmt.Errorf("error parsing http response from replay server: %v", err)
	}

	return res, nil
}

func (mm *MahiMahiTransport) Validate() error {
	if len(mm.WorkingDir) == 0 {
		return fmt.Errorf("no workingdir set")
	}

	if len(mm.RecordingDir) == 0 {
		return fmt.Errorf("no recordingdir set")
	}

	if len(mm.ReplayServerBin) == 0 {
		return fmt.Errorf("no replay server bin set")
	}

	return nil
}

func (mm *MahiMahiTransport) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
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

				mm.WorkingDir = p
			case "recording":
				if !d.NextArg() {
					return d.ArgErr()
				}

				p := d.Val()
				p = path.Clean(p)
				if path.IsAbs(p) {
					return d.Errf("%s can't be an absolute path", p)
				}

				mm.RecordingDir = p
			case "replayserver":
				if !d.Args(&mm.ReplayServerBin) {
					return d.ArgErr()
				}
			}

		}
	}

	return nil
}

var (
	_ http.RoundTripper = (*MahiMahiTransport)(nil)
)
