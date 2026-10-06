package loop

import (
	"bytes"
	"fmt"
	"strconv"
)

func parseRESP(buf []byte) (args []string, rest []byte, ok bool, err error) {
	if len(buf) == 0 {
		return nil, buf, false, nil
	}
	if buf[0] != '*' {
		return parseInline(buf)
	}
	nl := bytes.Index(buf, []byte("\r\n"))
	if nl < 0 {
		return nil, buf, false, nil
	}
	n, err := strconv.Atoi(string(buf[1:nl]))
	if err != nil || n < 0 {
		return nil, buf, false, fmt.Errorf("protocol error")
	}
	off := nl + 2
	args = make([]string, 0, n)
	for i := 0; i < n; i++ {
		if off >= len(buf) {
			return nil, buf, false, nil
		}
		if buf[off] != '$' {
			return nil, buf, false, fmt.Errorf("protocol error")
		}
		nl = bytes.Index(buf[off:], []byte("\r\n"))
		if nl < 0 {
			return nil, buf, false, nil
		}
		blen, err := strconv.Atoi(string(buf[off+1 : off+nl]))
		if err != nil || blen < 0 {
			return nil, buf, false, fmt.Errorf("protocol error")
		}
		off += nl + 2
		if off+blen+2 > len(buf) {
			return nil, buf, false, nil
		}
		if buf[off+blen] != '\r' || buf[off+blen+1] != '\n' {
			return nil, buf, false, fmt.Errorf("protocol error")
		}
		args = append(args, string(buf[off:off+blen]))
		off += blen + 2
	}
	return args, buf[off:], true, nil
}

func parseInline(buf []byte) (args []string, rest []byte, ok bool, err error) {
	nl := bytes.Index(buf, []byte("\r\n"))
	if nl < 0 {
		return nil, buf, false, nil
	}
	line := string(bytes.TrimSpace(buf[:nl]))
	if line == "" {
		return nil, buf[nl+2:], true, nil
	}
	fields := splitWS(line)
	return fields, buf[nl+2:], true, nil
}

func splitWS(s string) []string {
	var out []string
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(s[i])
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func bulk(s string) []byte {
	return []byte("$" + strconv.Itoa(len(s)) + "\r\n" + s + "\r\n")
}

func nilBulk() []byte { return []byte("$-1\r\n") }

func simple(s string) []byte { return []byte("+" + s + "\r\n") }

func integer(n int64) []byte { return []byte(":" + strconv.FormatInt(n, 10) + "\r\n") }

func errStr(s string) []byte { return []byte("-ERR " + s + "\r\n") }

func wrongType() []byte {
	return []byte("-WRONGTYPE Operation against a key holding the wrong kind of value\r\n")
}
