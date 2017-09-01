package crud

import (
	"strings"

	"ekt.com/ekt/x/safemap"
)

var (
	fullTitles         = []string{"API", "ASCII", "CPU", "CSS", "DNS", "EOF", "GUID", "HTML", "HTTP", "HTTPS", "ID", "IP", "JSON", "LHS", "QPS", "RAM", "RHS", "RPC", "SLA", "SMTP", "SSH", "TLS", "TTL", "UI", "UID", "UUID", "URI", "URL", "UTF8", "VM", "XML", "XSRF", "XSS", "PY"}
	fullTitlesReplacer *strings.Replacer
	//m和rm公用同一个
	dbNameMap = safemap.NewMapStringString()
)

func init() {
	var oldnew []string
	for _, title := range fullTitles {
		oldnew = append(oldnew, title, "_"+strings.ToLower(title))
	}
	for i := 'A'; i < 'Z'; i++ {
		oldnew = append(oldnew, string(i), "_"+string(i+32))
	}
	fullTitlesReplacer = strings.NewReplacer(oldnew...)
}

//ToDBName 将结构体的字段名字转换成对应数据库字段名
//比gorm速度快一倍
func ToDBName(name string) string {
	val, ok := dbNameMap.Get(name)
	if ok {
		return val
	}
	return toDBName(name)
}

//ToStructName 数据库字段名转换成对应结构体名
func ToStructName(name string) string {
	val, ok := dbNameMap.Get(name)
	if ok {
		return val
	}
	return ""
}

func toDBName(name string) string {
	dbName := fullTitlesReplacer.Replace(name)
	if len(dbName) >= 1 {
		dbNameMap.Set(name, dbName[1:])
		dbNameMap.Set(dbName[1:], name)
		return dbName[1:]
	}
	return ""
}
func ksvs(m map[string]interface{}, keyTail ...string) ([]string, []interface{}) {
	kt := ""
	ks := []string{}
	vs := []interface{}{}
	if len(keyTail) > 0 {
		kt = keyTail[0]
	}
	for k, v := range m {
		ks = append(ks, " `"+k+"`"+kt)
		vs = append(vs, v)
	}
	return ks, vs
}

func argslice(l int) string {
	s := []string{}
	for i := 0; i < l; i++ {
		s = append(s, "?")
	}
	return strings.Join(s, ",")
}
