// Package presenter はユースケースの結果を人間可読テキストと JSON に変換する。
package presenter

import (
	"bytes"
	"encoding/json"
	"io"
)

// KV は Object の 1 要素。
type KV struct {
	Key   string
	Value any
}

// Object はキーの挿入順を保つ JSON オブジェクト。元実装（Python の dict）の出力順と揃えるために使う。
type Object []KV

// MarshalJSON はキーの順序を保って書き出す。
func (o Object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, kv := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, err := marshalNoEscape(kv.Key)
		if err != nil {
			return nil, err
		}
		buf.Write(k)
		buf.WriteByte(':')
		v, err := marshalNoEscape(kv.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(v)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// WriteJSON は v をインデント 2・HTML エスケープなしで w に書き出す（json.dumps(indent=2, ensure_ascii=False) 相当）。
func WriteJSON(w io.Writer, v any) error {
	raw, err := marshalNoEscape(v)
	if err != nil {
		return err
	}
	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err != nil {
		return err
	}
	out.WriteByte('\n')
	_, err = w.Write(out.Bytes())
	return err
}

// IntsOrNull は nil を null、それ以外を配列として書き出すための値を返す。
func IntsOrNull(xs []int) any {
	if xs == nil {
		return nil
	}
	return xs
}

// StringOrNull は空文字を null として書き出すための値を返す。
func StringOrNull(s string) any {
	if s == "" {
		return nil
	}
	return s
}
