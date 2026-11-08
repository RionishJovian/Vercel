package routeros

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func sftpMockReply(t *testing.T, data string) *bytes.Buffer {
	t.Helper()
	var reply bytes.Buffer
	var version [4]byte
	binary.BigEndian.PutUint32(version[:], 3)
	if err := sftpWrite(&reply, sftpVersion, version[:]); err != nil { t.Fatal(err) }
	if err := sftpRequest(&reply, sftpHandle, 1, sftpString([]byte("handle"))); err != nil { t.Fatal(err) }
	if err := sftpRequest(&reply, sftpData, 2, sftpString([]byte(data))); err != nil { t.Fatal(err) }
	var eof [4]byte
	binary.BigEndian.PutUint32(eof[:], 1)
	if err := sftpRequest(&reply, sftpStatus, 3, eof[:]); err != nil { t.Fatal(err) }
	return &reply
}

func TestSFTPDownloadBoundedReadOnly(t *testing.T) {
	var requests bytes.Buffer
	payload, err := sftpDownload(&requests, sftpMockReply(t, "router-snapshot"), "demo.backup", 1024)
	if err != nil { t.Fatal(err) }
	if string(payload) != "router-snapshot" { t.Fatal("downloaded bytes mismatch") }
	if !bytes.Contains(requests.Bytes(), []byte("demo.backup")) { t.Fatal("filename missing in open request") }
	// All writes must be INIT, OPEN, READ and CLOSE: no upload/write/removal.
	types := map[byte]int{}
	r := bytes.NewReader(requests.Bytes())
	for r.Len() > 0 {
		typ, _, err := sftpNext(r)
		if err != nil { t.Fatal(err) }
		types[typ]++
	}
	if types[sftpInit] != 1 || types[sftpOpen] != 1 || types[sftpRead] != 2 || types[sftpClose] != 1 {
		t.Fatalf("unexpected SFTP operations: %v", types)
	}
}

func TestSFTPDownloadRejectsMalformedFrames(t *testing.T) {
	var requests bytes.Buffer
	buf := sftpMockReply(t, "data")
	raw := buf.Bytes()
	// A server must never allocate arbitrarily large packet bodies.
	var huge bytes.Buffer
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], sftpMaxFrame+1)
	huge.Write(header[:])
	if _, _, err := sftpNext(&huge); err == nil {
		t.Fatal("oversized response must be rejected")
	}
	// Corrupt the reply id of the first OPEN response.
	corrupt := append([]byte(nil), raw...)
	// version frame consumes 9 bytes (4 length + 1 type + 4 version).
	if len(corrupt) < 15 { t.Fatal("test fixture too short") }
	corrupt[14] ^= 0xff
	if _, err := sftpDownload(&requests, bytes.NewReader(corrupt), "demo.backup", 1024); err == nil {
		t.Fatal("wrong SFTP response id must fail")
	}
	if _, err := sftpDownload(&requests, sftpMockReply(t, strings.Repeat("a", 30)), "demo.backup", 10); err == nil {
		t.Fatal("SFTP download cannot exceed configured size")
	}
}
