package peermanager

import (
	"bytes"
	"context"
	"crypto/sha512"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/xrpl/relay-server/internal/identity"
	"github.com/xrpl/relay-server/internal/token"
)

type ConnStatus int32

const (
	ConnOK ConnStatus = iota
	ConnFull
	ConnError
)

func (cs ConnStatus) String() string {
	switch cs {
	case ConnOK:
		return "ok"
	case ConnFull:
		return "no_slots"
	case ConnError:
		return "error"
	default:
		return "invalid_status"
	}
}

type ConnMetadata struct {
	ConnStatus     ConnStatus
	Server         string
	Version        string
	NetworkID      int32
	PublicKey      token.PublicKey
	SessionSig     string
	InstanceCookie int64
	Domain         string
	RemoteIP       net.IP
	PeerIPs        []string
	Response       []byte
}

func connect(addr string) (*tls.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		return nil, err
	}

	// To prevent MITM attacks, the XRP Ledger uses the TLS/SSL handshake values ClientFinished and ServerFinished
	// for validation. In C++, these values are easily accessible, but in Go, they are private and hidden.
	// The only way to access these values in Go is by enforcing TLS 1.2 on both the client and server
	// and using reflection to extract the private fields containing the ClientFinished and ServerFinished values.
	tlsConn := tls.Client(conn, &tls.Config{
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS12,

		InsecureSkipVerify: true,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = tlsConn.HandshakeContext(ctx)
	if err != nil {
		conn.Close()
		return nil, err
	}

	return tlsConn, nil
}

func upgrade(conn *tls.Conn, i *identity.Identity) (*ConnMetadata, error) {
	sharedValue := tlsCookie(conn)
	sig, err := sessionSig(i, sharedValue)
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			conn.Close()
		}
	}()

	builder := &bytes.Buffer{}
	builder.WriteString("GET / HTTP/1.1\r\n")
	builder.WriteString("User-Agent: relay-0.0.1a\r\n")
	builder.WriteString("Upgrade: RTXP/1.2, XRPL/2.0, XRPL/2.1\r\n")
	builder.WriteString("Connection: Upgrade\r\n")
	builder.WriteString("Connect-As: Peer\r\n")
	builder.WriteString("Crawl: private\r\n")
	builder.WriteString("Session-Signature: " + sig + "\r\n")
	builder.WriteString("Public-Key: " + string(i.EncodedPublicKey()) + "\r\n\r\n")

	_, err = conn.Write(builder.Bytes())
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 6144)
	_, err = conn.Read(buf)
	if err != nil && !errors.Is(err, io.EOF) {
		fmt.Println(string(buf), err)
		return nil, err
	}

	md, err := parseHandshakeResponse(buf)
	if err != nil {
		return nil, err
	}

	if md.ConnStatus != ConnOK {
		return md, nil
	}

	// verify the information passed

	if !md.PublicKey.VerifyKeyType(token.SECP256k1) {
		err = errors.New("unsupported public key")
		return nil, err
	}

	if len(md.SessionSig) == 0 {
		return nil, errors.New("missing session signature")
	}

	remoteSig, err := base64.StdEncoding.DecodeString(md.SessionSig)
	if err != nil {
		return nil, err
	}

	ok, err := md.PublicKey.VerifySignature(sharedValue, remoteSig)
	if err != nil {
		return nil, errors.Join(errors.New("failed to verify session signature"), err)
	}

	if !ok {
		return nil, errors.New("invalid session signature")
	}

	return md, nil
}

func sessionSig(i *identity.Identity, sharedValue []byte) (string, error) {
	sig, err := i.Sign(sharedValue)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(sig), nil
}

func tlsCookie(conn *tls.Conn) []byte {
	sh := sha512.New()
	sh.Write(reflect.ValueOf(conn).Elem().FieldByName("clientFinished").Bytes())
	cookie1 := sh.Sum(nil)

	sh.Reset()
	sh.Write(reflect.ValueOf(conn).Elem().FieldByName("serverFinished").Bytes())
	cookie2 := sh.Sum(nil)

	// this should never hit. Sanity check
	if len(cookie1) != len(cookie2) {
		panic("server & client secret values are not equal")
	}

	out := make([]byte, len(cookie1))
	for i := range cookie2 {
		out[i] = cookie1[i] ^ cookie2[i]
	}

	sh.Reset()
	sh.Write(out)

	copy(out, sh.Sum(nil))

	return out
}

func parseHandshakeResponse(response []byte) (*ConnMetadata, error) {
	md := &ConnMetadata{}
	md.Response = response
	lines := bytes.Split(response, []byte("\n"))
	if len(lines) < 1 {
		return nil, errors.New("invalid header response: " + string(response))
	}

	for _, line := range lines[1:] {

		if bytes.Contains(line, []byte("peer-ips")) {
			m := map[string][]string{}
			err := json.Unmarshal(line, &m)
			if err != nil {
				println(err)
			}

			md.PeerIPs = m["peer-ips"]
			continue
		}

		idx := bytes.Index(line, []byte(":"))
		if idx == -1 {
			continue
		}

		key := line[:idx]

		value := strings.Trim(strings.TrimSpace(string(line[idx+2:])), "\n")

		switch strings.ToLower(string(key)) {
		case "network-id":
			if id, err := strconv.ParseInt(string(value), 10, 32); err == nil {
				md.NetworkID = int32(id)
			}
		case "public-key":
			rawKey, err := token.Decode[token.PublicKey](token.PublicNode, value)
			if err != nil {
				return nil, err
			}
			md.PublicKey = rawKey
		case "session-signature":
			md.SessionSig = value
		case "instance-cookie":
			if cookie, err := strconv.ParseInt(string(value), 10, 64); err == nil {
				md.InstanceCookie = cookie
			}
		case "server-domain":
			md.Domain = value
		case "remote-ip":
			if ip := net.ParseIP(value); ip != nil {
				md.RemoteIP = ip
			}
		case "server":
			md.Server = value
		}
	}

	if bytes.Contains(lines[0], []byte("101")) {
		md.ConnStatus = ConnOK
	} else if bytes.Contains(lines[0], []byte("503")) {
		if strings.Contains(md.Server, "rippled") {
			// we got a response from rippled server, it's probably ok
			md.ConnStatus = ConnFull
		} else {
			fmt.Println(string(response))
		}
	} else {
		md.ConnStatus = ConnError
		fmt.Println(string(response))
	}

	return md, nil
}
