package routeros

// Minimal read-only SFTP v3 client, scoped to downloading two router backup
// artifacts. Reuses the existing pinned SSH configuration; no shell commands,
// SSH agent, SCP command injection, uploads or remote file deletion.
import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"

)

const (
	sftpInit   byte = 1
	sftpVersion byte = 2
	sftpOpen   byte = 3
	sftpClose  byte = 4
	sftpRead   byte = 5
	sftpStatus byte = 101
	sftpHandle byte = 102
	sftpData   byte = 103
	sftpMaxFrame = 1 << 20
	sftpReadSize = 32768
)

// DownloadFile reads a single named RouterOS file over a pinned SSH SFTP
// subsystem. The maximum size is enforced while reading (not only after).
// The caller must validate filename, expected size and cryptographic digest.
func (t *SSHTransport) DownloadFile(ctx context.Context, filename string, maxBytes int64) ([]byte, error) {
	if filename == "" || strings.ContainsAny(filename, "/\\\x00\r\n") {
		return nil, errors.New("routeros: invalid artifact filename")
	}
	if maxBytes <= 0 || maxBytes > 32<<20 {
		return nil, errors.New("routeros: invalid artifact size limit")
	}
	client, err := t.connect(ctx)
	if err != nil { return nil, err }
	defer client.Close()
	stop := context.AfterFunc(ctx, func() { _ = client.Close() })
	defer stop()
	session, err := client.NewSession()
	if err != nil { return nil, fmt.Errorf("routeros: sftp session unavailable: %w", err) }
	defer session.Close()
	in, err := session.StdinPipe()
	if err != nil { return nil, errors.New("routeros: sftp input unavailable") }
	out, err := session.StdoutPipe()
	if err != nil { return nil, errors.New("routeros: sftp output unavailable") }
	if err := session.RequestSubsystem("sftp"); err != nil {
		return nil, errors.New("routeros: sftp subsystem unavailable")
	}
	data, err := sftpDownload(in, out, filename, maxBytes)
	if err != nil { return nil, err }
	if ctx.Err() != nil { return nil, ctx.Err() }
	return data, nil
}

func sftpString(b []byte) []byte {
	var p [4]byte
	binary.BigEndian.PutUint32(p[:], uint32(len(b)))
	return append(p[:], b...)
}

func sftpWrite(w io.Writer, typ byte, payload []byte) error {
	var length [4]byte
	binary.BigEndian.PutUint32(length[:], uint32(1+len(payload)))
	if _, err := w.Write(length[:]); err != nil { return err }
	if _, err := w.Write([]byte{typ}); err != nil { return err }
	_, err := w.Write(payload)
	return err
}

func sftpNext(r io.Reader) (byte, []byte, error) {
	var length [4]byte
	if _, err := io.ReadFull(r, length[:]); err != nil { return 0, nil, err }
	n := binary.BigEndian.Uint32(length[:])
	if n == 0 || n > sftpMaxFrame {
		return 0, nil, errors.New("routeros: invalid SFTP frame length")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil { return 0, nil, err }
	return buf[0], buf[1:], nil
}

func sftpField(buf []byte) ([]byte, []byte, error) {
	if len(buf) < 4 { return nil, nil, errors.New("routeros: truncated SFTP field") }
	n := binary.BigEndian.Uint32(buf)
	if n > uint32(len(buf)-4) { return nil, nil, errors.New("routeros: invalid SFTP field length") }
	return buf[4:4+n], buf[4+n:], nil
}

func sftpRequest(w io.Writer, typ byte, id uint32, payload []byte) error {
	var head [4]byte
	binary.BigEndian.PutUint32(head[:], id)
	return sftpWrite(w, typ, append(head[:], payload...))
}

func sftpResponse(r io.Reader, id uint32) (byte, []byte, error) {
	typ, payload, err := sftpNext(r)
	if err != nil { return 0, nil, err }
	if len(payload) < 4 || binary.BigEndian.Uint32(payload[:4]) != id {
		return 0, nil, errors.New("routeros: unexpected SFTP response id")
	}
	return typ, payload[4:], nil
}

// sftpDownload is intentionally a tiny audited subset of SFTP v3. RouterOS
// host-key and user authentication are handled by SSHTransport before entry.
func sftpDownload(w io.Writer, r io.Reader, filename string, maxBytes int64) ([]byte, error) {
	var ver [4]byte
	binary.BigEndian.PutUint32(ver[:], 3)
	if err := sftpWrite(w, sftpInit, ver[:]); err != nil { return nil, err }
	typ, resp, err := sftpNext(r)
	if err != nil { return nil, err }
	if typ != sftpVersion || len(resp) < 4 || binary.BigEndian.Uint32(resp[:4]) != 3 {
		return nil, errors.New("routeros: SFTP v3 not supported")
	}
	// SSH_FXF_READ = 1; attrs.flags = 0.
	open := append(sftpString([]byte(filename)), 0,0,0,1, 0,0,0,0)
	if err := sftpRequest(w, sftpOpen, 1, open); err != nil { return nil, err }
	typ, resp, err = sftpResponse(r, 1)
	if err != nil { return nil, err }
	if typ != sftpHandle { return nil, errors.New("routeros: SFTP file open refused") }
	handle, _, err := sftpField(resp)
	if err != nil || len(handle) == 0 { return nil, errors.New("routeros: invalid SFTP handle") }
	defer func() { _ = sftpRequest(w, sftpClose, 0xfffffffe, sftpString(handle)) }()
	var dst bytes.Buffer
	for id := uint32(2); ; id++ {
		if int64(dst.Len()) >= maxBytes { return nil, errors.New("routeros: artifact exceeds configured limit") }
		chunk := int64(sftpReadSize)
		if maxBytes-int64(dst.Len()) < chunk { chunk = maxBytes-int64(dst.Len()) }
		var off [8]byte
		binary.BigEndian.PutUint64(off[:], uint64(dst.Len()))
		var sz [4]byte
		binary.BigEndian.PutUint32(sz[:], uint32(chunk))
		req := append(sftpString(handle), off[:]...)
		req = append(req, sz[:]...)
		if err := sftpRequest(w, sftpRead, id, req); err != nil { return nil, err }
		typ, resp, err := sftpResponse(r, id)
		if err != nil { return nil, err }
		if typ == sftpStatus {
			if len(resp) >= 4 && binary.BigEndian.Uint32(resp[:4]) == 1 { break } // SSH_FX_EOF
			return nil, errors.New("routeros: SFTP download failed")
		}
		if typ != sftpData { return nil, errors.New("routeros: unexpected SFTP data response") }
		part, _, err := sftpField(resp)
		if err != nil || len(part) == 0 || int64(len(part)) > chunk {
			return nil, errors.New("routeros: invalid SFTP data chunk")
		}
		dst.Write(part)
	}
	if dst.Len() == 0 { return nil, errors.New("routeros: downloaded backup is empty") }
	return dst.Bytes(), nil
}

var _ interface {
	DownloadFile(context.Context, string, int64) ([]byte, error)
} = (*SSHTransport)(nil)

