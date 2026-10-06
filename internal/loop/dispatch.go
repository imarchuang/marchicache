package loop

import (
	"strconv"
	"strings"
	"time"

	"github.com/marchi/marchicache/internal/store"
)

func dispatch(st *store.Store, args []string) []byte {
	if len(args) == 0 {
		return errStr("empty command")
	}
	cmd := strings.ToUpper(args[0])
	switch cmd {
	case "PING":
		if len(args) == 2 {
			return bulk(args[1])
		}
		return simple("PONG")
	case "COMMAND":
		return simple("OK")
	case "GET":
		if len(args) != 2 {
			return errStr("wrong number of arguments for 'get' command")
		}
		v, ok, err := st.Get(args[1])
		if err == store.ErrWrongType {
			return wrongType()
		}
		if !ok {
			return nilBulk()
		}
		return bulk(v)
	case "SET":
		if len(args) < 3 {
			return errStr("wrong number of arguments for 'set' command")
		}
		var ttl time.Duration
		if len(args) >= 5 && strings.EqualFold(args[3], "EX") {
			sec, err := strconv.ParseInt(args[4], 10, 64)
			if err != nil {
				return errStr("value is not an integer or out of range")
			}
			ttl = time.Duration(sec) * time.Second
		} else if len(args) != 3 {
			return errStr("syntax error")
		}
		if ttl > 0 {
			st.SetEX(args[1], args[2], ttl)
		} else {
			st.Set(args[1], args[2])
		}
		return simple("OK")
	case "DEL":
		if len(args) != 2 {
			return errStr("wrong number of arguments for 'del' command")
		}
		if st.Del(args[1]) {
			return integer(1)
		}
		return integer(0)
	case "EXPIRE":
		if len(args) != 3 {
			return errStr("wrong number of arguments for 'expire' command")
		}
		sec, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil {
			return errStr("value is not an integer or out of range")
		}
		if st.Expire(args[1], time.Duration(sec)*time.Second) {
			return integer(1)
		}
		return integer(0)
	case "TTL":
		if len(args) != 2 {
			return errStr("wrong number of arguments for 'ttl' command")
		}
		return integer(st.TTL(args[1]))
	case "HSET":
		if len(args) != 4 {
			return errStr("wrong number of arguments for 'hset' command")
		}
		n, err := st.HSet(args[1], args[2], args[3])
		if err == store.ErrWrongType {
			return wrongType()
		}
		return integer(int64(n))
	case "HGET":
		if len(args) != 3 {
			return errStr("wrong number of arguments for 'hget' command")
		}
		v, ok, err := st.HGet(args[1], args[2])
		if err == store.ErrWrongType {
			return wrongType()
		}
		if !ok {
			return nilBulk()
		}
		return bulk(v)
	case "HGETALL":
		if len(args) != 2 {
			return errStr("wrong number of arguments for 'hgetall' command")
		}
		all, err := st.HGetAll(args[1])
		if err == store.ErrWrongType {
			return wrongType()
		}
		var b []byte
		b = append(b, '*')
		b = append(b, strconv.Itoa(len(all)*2)...)
		b = append(b, "\r\n"...)
		for f, v := range all {
			b = append(b, bulk(f)...)
			b = append(b, bulk(v)...)
		}
		return b
	default:
		return errStr("unknown command '" + args[0] + "'")
	}
}
