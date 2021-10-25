package smtp

import (
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strings"
)

// A Client represents a client Connection to an SMTP server.
type Client struct {
	// Text is the textproto.Conn used by the Client. It is exported to allow for
	// clients to add extensions.
	Text *textproto.Conn
	// keep a reference to the Connection so it can be used to create a TLS
	// Connection later
	Conn net.Conn
	// whether the Client is using TLS
	tls bool
	// ServerName server name for smtp
	ServerName string
	// map of supported extensions
	ext map[string]string
	// supported auth mechanisms
	auth []string
	// Response Code on calling .
	ResponseCode int
	//  Response of every call
	ResponseMessage string
	serverName      string
}

// Dial returns a new Client Connected to an SMTP server at addr.
func Dial(addr string, ptr string) (*Client, error) {
	if !strings.HasSuffix(addr, ":25") {
		addr = fmt.Sprintf("%s:%d", strings.TrimSuffix(addr, "."), 25)
	}
	Conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	host := addr[:strings.Index(addr, ":")]
	return NewClient(Conn, host, ptr)
}

// NewClient returns a new Client using an existing Connection and host as a
// server name to be used when authenticating.
func NewClient(Conn net.Conn, host string, ptr string) (*Client, error) {
	text := textproto.NewConn(Conn)

	_, msg, err := text.ReadResponse(220)
	if err != nil {
		text.Close()
		return nil, err
	}
	c := &Client{Text: text, Conn: Conn, ServerName: host}
	if strings.Contains(msg, "ESMTP") {
		err = c.ehlo(ptr)
	} else {
		err = c.helo(ptr)
	}
	return c, err
}

// cmd is a convenience function that sends a command and returns the response
func (c *Client) cmd(expectCode int, format string, args ...interface{}) (int, string, error) {
	id, err := c.Text.Cmd(format, args...)
	if err != nil {
		return 0, "", err
	}
	c.Text.StartResponse(id)
	defer c.Text.EndResponse(id)
	code, msg, err := c.Text.ReadResponse(expectCode)
	return code, msg, err
}

// helo sends the HELO greeting to the server. It should be used only when the
// server does not support ehlo.
func (c *Client) helo(ptr string) error {
	c.ext = nil
	_, _, err := c.cmd(250, "HELO "+ptr)
	return err
}

// ehlo sends the EHLO (extended hello) greeting to the server. It
// should be the preferred greeting for servers that support it.
func (c *Client) ehlo(ptr string) error {
	_, msg, err := c.cmd(250, "EHLO "+ptr)
	if err != nil {
		return err
	}
	ext := make(map[string]string)
	extList := strings.Split(msg, "\n")
	if len(extList) > 1 {
		extList = extList[1:]
		for _, line := range extList {
			args := strings.SplitN(line, " ", 2)
			if len(args) > 1 {
				ext[args[0]] = args[1]
			} else {
				ext[args[0]] = ""
			}
		}
	}
	if mechs, ok := ext["AUTH"]; ok {
		c.auth = strings.Split(mechs, " ")
	}
	c.ext = ext
	return err
}

// StartTLS sends the STARTTLS command and encrypts all further communication.
// Only servers that advertise the STARTTLS extension support this function.
func (c *Client) StartTLS(config *tls.Config, ptr string) error {
	_, _, err := c.cmd(220, "STARTTLS")
	if err != nil {
		return err
	}
	c.Conn = tls.Client(c.Conn, config)
	c.Text = textproto.NewConn(c.Conn)
	c.tls = true
	return c.ehlo(ptr)
}

// Verify checks the validity of an email address on the server.
// If Verify returns nil, the address is valid. A non-nil return
// does not necessarily indicate an invalid address. Many servers
// will not verify addresses for security reasons.
func (c *Client) Verify(addr string) error {
	_, _, err := c.cmd(250, "VRFY %s", addr)
	return err
}

// Mail issues a MAIL command to the server using the provided email address.
// If the server supports the 8BITMIME extension, Mail adds the BODY=8BITMIME
// parameter.
// This initiates a mail transaction and is followed by one or more Rcpt calls.
func (c *Client) Mail(from string) error {
	c.ResponseMessage = ""
	cmdStr := "MAIL FROM:<%s>"
	if c.ext != nil {
		if _, ok := c.ext["8BITMIME"]; ok {
			cmdStr += " BODY=8BITMIME"
		}
	}
	_, _, err := c.cmd(250, cmdStr, from)
	return err
}

// Rcpt issues a RCPT command to the server using the provided email address.
// A call to Rcpt must be preceded by a call to Mail and may be followed by
// a Data call or another Rcpt call.
func (c *Client) Rcpt(to string) error {
	_, _, err := c.cmd(25, "RCPT TO:<%s>", to)
	return err
}

type dataCloser struct {
	c *Client
	io.WriteCloser
}

func (d *dataCloser) Close() error {
	d.WriteCloser.Close()
	code, msg, err := d.c.Text.ReadResponse(250)
	d.c.ResponseMessage = msg
	d.c.ResponseCode = code
	return err
}

// Data issues a DATA command to the server and returns a writer that
// can be used to write the data. The caller should close the writer
// before calling any more methods on c.
// A call to Data must be preceded by one or more calls to Rcpt.
func (c *Client) Data() (io.WriteCloser, error) {
	_, _, err := c.cmd(354, "DATA")
	if err != nil {
		return nil, err
	}
	return &dataCloser{c, c.Text.DotWriter()}, nil
}

// Extension reports whether an extension is support by the server.
// The extension name is case-insensitive. If the extension is supported,
// Extension also returns a string that contains any parameters the
// server specifies for the extension.
func (c *Client) Extension(ext string) (bool, string) {
	if c.ext == nil {
		return false, ""
	}
	ext = strings.ToUpper(ext)
	param, ok := c.ext[ext]
	return ok, param
}

// Reset sends the RSET command to the server, aborting the current mail
// transaction.
func (c *Client) Reset() error {
	_, _, err := c.cmd(250, "RSET")
	return err
}

// Quit sends the QUIT command and closes the Connection to the server.
func (c *Client) Quit() error {
	_, _, err := c.cmd(221, "QUIT")
	if err != nil {
		return err
	}
	return c.Text.Close()
}

// Close closes the Connection.
func (c *Client) Close() error {
	return c.Text.Close()
}

// ClearMsg sets the str = ""
func (c *Client) ClearMsg() {
	c.ResponseMessage = ""
}

func (c *Client) Noop(ptr string) error {
	if err := c.helo(ptr); err != nil {
		return err
	}
	_, _, err := c.cmd(250, "NOOP")
	return err
}

func (c *Client) Auth(a Auth) error {
	if err := c.helo("localhost"); err != nil {
		return err
	}
	encoding := base64.StdEncoding
	mech, resp, err := a.Start(&ServerInfo{c.serverName, c.tls, c.auth})
	if err != nil {
		c.Quit()
		return err
	}
	resp64 := make([]byte, encoding.EncodedLen(len(resp)))
	encoding.Encode(resp64, resp)
	code, msg64, err := c.cmd(0, strings.TrimSpace(fmt.Sprintf("AUTH %s %s", mech, resp64)))
	for err == nil {
		var msg []byte
		switch code {
		case 334:
			msg, err = encoding.DecodeString(msg64)
		case 235:
			// the last message isn't base64 because it isn't a challenge
			msg = []byte(msg64)
		default:
			err = &textproto.Error{Code: code, Msg: msg64}
		}
		if err == nil {
			resp, err = a.Next(msg, code == 334)
		}
		if err != nil {
			// abort the AUTH
			c.cmd(501, "*")
			c.Quit()
			break
		}
		if resp == nil {
			break
		}
		resp64 = make([]byte, encoding.EncodedLen(len(resp)))
		encoding.Encode(resp64, resp)
		code, msg64, err = c.cmd(0, string(resp64))
	}
	return err
}
