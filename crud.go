package crud

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"time"
)

// 错误
var (
	ErrExec = errors.New("执行错误")
	ErrArgs = errors.New("参数错误")
)

// 用于编程经常遇到的时间
var (
	TimeFormat = "2006-01-02 15:04:05"
)

// Render 用于对接http.HandleFunc直接调用CRUD
type Render func(w http.ResponseWriter, err error, data ...interface{})

// Columns 用于表示一张表中的列，使用名字作为index，方便查找。
type Columns map[string]Column

// HaveColumn 是否有此类
func (cs Columns) HaveColumn(columnName string) bool {
	if cs == nil {
		return false
	}
	_, ok := cs[columnName]
	return ok
}

// Column 是描述一个具体的列
type Column struct {
	Name       string
	Comment    string
	ColumnType string
	DataType   string
}

// CRUD 本包关键类
type CRUD struct {
	debug bool

	tableColumns   map[string]Columns
	dataSourceName string
	p              *Pools
	render         Render //crud本身不渲染数据，通过其他地方传入一个渲染的函数，然后渲染都是那边处理。
}

// NewCRUD 创建一个新的CRUD链接
func NewCRUD(dataSourceName string, render ...Render) *CRUD {
	crud := &CRUD{
		debug:          false,
		tableColumns:   make(map[string]Columns),
		dataSourceName: dataSourceName,
		p:              NewPool(dataSourceName, 20),
		render: func(w http.ResponseWriter, err error, data ...interface{}) {
			if len(render) == 1 {
				if render[0] != nil {
					render[0](w, err, data...)
				}
			}
		},
	}

	for _, tbm := range crud.Query("SHOW TABLES").RawsMap() {
		for _, v := range tbm {
			crud.getColums(v)
		}
	}
	return crud
}

// Table 返回一个Table
func (api *CRUD) Table(tablename string) *Table {
	return &Table{CRUD: api, tableName: tablename}
}

// ExecSuccessRender 渲染成功模板
func (api *CRUD) ExecSuccessRender(w http.ResponseWriter) {
	api.render(w, nil, nil)
}

func (api *CRUD) argsErrorRender(w http.ResponseWriter) {
	api.render(w, ErrArgs)
}

func (api *CRUD) execErrorRender(w http.ResponseWriter) {
	api.render(w, ErrExec)
}

func (api *CRUD) dataRender(w http.ResponseWriter, data interface{}) {
	api.render(w, nil, data)
}

// 是否有这张表名
func (api *CRUD) haveTablename(tableName string) bool {
	_, ok := api.tableColumns[tableName]
	return ok
}

// 获取表中所有列名
func (api *CRUD) getColums(tablename string) Columns {
	names, ok := api.tableColumns[tablename]
	if ok {
		return names
	}
	raws := api.Query("SELECT COLUMN_NAME,COLUMN_COMMENT,COLUMN_TYPE,DATA_TYPE FROM information_schema.`COLUMNS` WHERE table_name= ? ", tablename).RawsMap()
	cols := make(map[string]Column)
	for _, v := range raws {
		cols[v["COLUMN_NAME"]] = Column{Name: v["COLUMN_NAME"], Comment: v["COLUMN_COMMENT"], ColumnType: v["COLUMN_TYPE"], DataType: v["DATA_TYPE"]}
		DBColums[v["COLUMN_NAME"]] = cols[v["COLUMN_NAME"]]
	}
	api.tableColumns[tablename] = cols
	return cols
}

// Debug 是否开启debug功能 true为开启
func (api *CRUD) Debug(isDebug bool) *CRUD {
	api.debug = isDebug
	return api
}

// X 用于DEBUG
func (*CRUD) X(args ...interface{}) {
	fmt.Println("[DEBUG]", args)
}

// Log 打印日志
func (api *CRUD) Log(args ...interface{}) {
	if api.debug {
		api.log(args...)
	}
}

func (api *CRUD) log(args ...interface{}) {
	log.Println(args...)
}

// LogSQL 会将sql语句中的?替换成相应的参数，让DEBUG的时候可以直接复制SQL语句去使用。
func (api *CRUD) LogSQL(sql string, args ...interface{}) {
	if api.debug {
		api.logSQL(sql, args...)
	}
}

func (api *CRUD) logSQL(sql string, args ...interface{}) {
	for _, arg := range args {
		sql = strings.Replace(sql, "?", fmt.Sprintf("'%v'", arg), 1)
	}
	api.Log(sql)
}

// RowSQL 执行查询sqL
func (api *CRUD) RowSQL(sql string, args ...interface{}) *SQLRows {
	return api.Query(sql, args...)
}

// Query 用于底层查询，一般是SELECT语句
func (api *CRUD) Query(sql string, args ...interface{}) *SQLRows {
	db, err := api.DB()
	defer db.Close()
	if err != nil {
		api.Log("[ERROR]", err)
		return &SQLRows{}
	}
	api.LogSQL(sql, args...)
	rows, err := db.Query(sql, args...)
	/*
		dial tcp 192.168.2.14:3306: connectex: Only one usage of each socket address (protocol/network address/port) is normally permitted.
	*/
	if err != nil {
		fmt.Println(time.Now().Format(TimeFormat), err)
		oldDebug := api.debug
		api.debug = true
		api.LogSQL(sql, args...)
		api.debug = oldDebug

		//这里会打印调用栈
		buf := make([]byte, 1<<10)
		runtime.Stack(buf, true)
		log.Printf("\n%s", buf)

	}
	return &SQLRows{rows: rows, err: err}
}

// Exec 用于底层执行，一般是INSERT INTO、DELETE、UPDATE。
func (api *CRUD) Exec(sql string, args ...interface{}) *SQLResult {
	db, err := api.DB()
	defer db.Close()
	if err != nil {
		api.Log("[ERROR]", err)
		return &SQLResult{}
	}
	api.LogSQL(sql, args...)
	ret, err := db.Exec(sql, args...)
	if err != nil {
		fmt.Println(time.Now().Format(TimeFormat), err)
		oldDebug := api.debug
		api.debug = true
		api.LogSQL(sql, args...)
		api.debug = oldDebug

		//这里会打印调用栈
		buf := make([]byte, 1<<10)
		runtime.Stack(buf, true)
		log.Printf("\n%s", buf)
	}
	return &SQLResult{ret: ret, err: err}
}

// DB 返回一个打开的DB链接
func (api *CRUD) DB() (*DB, error) {
	return api.p.Open()
}

// Create 创建 旧的 TODO
/*
	Create 用于创建

*/
func (api *CRUD) Create(v interface{}, w http.ResponseWriter, r *http.Request) {
	tableName := getStructDBName(v)
	m := parseRequest(v, r, C)
	if m == nil || len(m) == 0 {
		api.argsErrorRender(w)
		return
	}
	names := []string{}
	values := []string{}
	args := []interface{}{}
	cols := api.getColums(tableName)
	if cols.HaveColumn("created_at") {
		m["created_at"] = time.Now().Format(TimeFormat)
	}
	if cols.HaveColumn("is_deleted") {
		m["is_deleted"] = 0
	}
	if cols.HaveColumn("updated_at") {
		m["updated_at"] = time.Now().Format(TimeFormat)
	}
	for k, v := range m {
		names = append(names, "`"+k+"`")
		values = append(values, "?")
		args = append(args, v)
	}
	ret := api.Exec(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", tableName, strings.Join(names, ","), strings.Join(values, ",")), args...)
	id, err := ret.ID()
	if err != nil {
		api.execErrorRender(w)
		return
	}
	m["id"] = id
	delete(m, "is_deleted")
	api.dataRender(w, m)
}

/*
	查找
	id = 1
	id = 1  AND hospital_id = 1

*/
func (api *CRUD) Read(v interface{}, w http.ResponseWriter, r *http.Request) {
	//	这里传进来的参数一定是要有用的参数，如果是没有用的参数被传进来了，那么会报参数错误，或者显示执行成功数据会乱。
	//	这里处理last_XXX
	//	处理翻页的问题
	//	首先判断这个里面有没有这个字段
	m := parseRequest(v, r, R)
	//	if m == nil || len(m) == 0 {
	//		api.argsErrorRender(w)
	//		return
	//	}
	//	看一下是不是其他表关联查找
	tableName := getStructDBName(v)
	cols := api.getColums(tableName)
	ctn := "" //combine table name
	//fk := ""
	//var fkv interface{}
	for k := range m {
		if !cols.HaveColumn(k) {
			if strings.Contains(k, "_id") {
				atn := strings.TrimRight(k, "_id") //another table name
				tmptn := atn + "_" + tableName
				api.X("检查表" + tmptn)
				if api.haveTablename(tmptn) {
					if api.tableColumns[tmptn].HaveColumn(k) {
						ctn = tmptn
					}
				}
				api.X("检查表" + tmptn)
				tmptn = tableName + "_" + atn
				if api.haveTablename(tmptn) {
					if api.tableColumns[tmptn].HaveColumn(k) {
						ctn = tmptn
					}
				}
				//				if ctn == "" {
				//					api.argsErrorRender(w)
				//					return
				//				}
			}
		}
	}
	if api.tableColumns[tableName].HaveColumn("is_deleted") {
		m["is_deleted"] = "0"
	}
	if ctn == "" {
		//如果没有设置ID，则查找所有的。
		if m == nil || len(m) == 0 {
			data := api.Query(fmt.Sprintf("SELECT * FROM `%s`", tableName)).RawsMapInterface()
			api.dataRender(w, data)
		} else {
			ks, vs := ksvs(m, " = ? ")
			data := api.Query(fmt.Sprintf("SELECT * FROM `%s` WHERE %s", tableName, strings.Join(ks, "AND")), vs...).RawsMapInterface()
			api.dataRender(w, data)
		}
	} else {
		ks, vs := ksvs(m, " = ? ")
		//SELECT `section`.* FROM `group_section` LEFT JOIN section ON group_section.section_id = section.id WHERE group_id = 1
		data := api.Query(fmt.Sprintf("SELECT `%s`.* FROM `%s` LEFT JOIN `%s` ON `%s`.`%s` = `%s`.`%s` WHERE %s", tableName, ctn, tableName, ctn, tableName+"_id", tableName, "id", strings.Join(ks, "AND")), vs...).RawsMapInterface()
		api.dataRender(w, data)
	}
}

// Update 更新
func (api *CRUD) Update(v interface{}, w http.ResponseWriter, r *http.Request) {
	fmt.Println(r.Form)
	tableName := getStructDBName(v)
	m := parseRequest(v, r, R)
	if m == nil || len(m) == 0 {
		api.argsErrorRender(w)
		return
	}
	//	UPDATE task SET name = ? WHERE id = 3;
	//	现在只支持根据ID进行更新
	id := m["id"]
	delete(m, "id")
	if api.tableColumns[tableName].HaveColumn("updated_at") {
		m["updated_at"] = time.Now().Format(TimeFormat)
	}
	ks, vs := ksvs(m, " = ? ")
	vs = append(vs, id)
	_, err := api.Exec(fmt.Sprintf("UPDATE `%s` SET %s WHERE %s = ?", tableName, strings.Join(ks, ","), "id"), vs...).Effected()
	if err != nil {
		api.execErrorRender(w)
		return
	}
	api.ExecSuccessRender(w)
}

// Delete 删除
func (api *CRUD) Delete(v interface{}, w http.ResponseWriter, r *http.Request) {
	tableName := getStructDBName(v)
	m := parseRequest(v, r, R)
	if m == nil || len(m) == 0 {
		api.argsErrorRender(w)
		return
	}
	if api.tableColumns[tableName].HaveColumn("is_deleted") {
		if api.tableColumns[tableName].HaveColumn("deleted_at") {
			r.Form["deleted_at"] = []string{time.Now().Format(TimeFormat)}
		}
		r.Form["is_deleted"] = []string{"1"}
		api.Update(v, w, r)
		return
	}
	//	现在只支持根据ID进行删除
	ks, vs := ksvs(m, " = ? ")
	_, err := api.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s ", tableName, strings.Join(ks, "AND")), vs...).Effected()
	if err != nil {
		api.execErrorRender(w)
		return
	}
	api.ExecSuccessRender(w)
}

// Find 将查找数据放到结构体里面
func (api *CRUD) Find(v interface{}, args ...interface{}) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		fmt.Println("Must Need Addr")
		return
	}

	if len(args) > 0 {
		if sql, ok := args[0].(string); ok {
			if strings.Contains(args[0].(string), "SELECT") {
				api.Query(sql, args[1:]...).Find(v)
				return
			}
		}
	}

	tableName := ""
	if rv.Elem().Kind() == reflect.Slice {
		tableName = ToDBName(rv.Elem().Type().Elem().Name())
	} else {
		tableName = ToDBName(rv.Type().Elem().Name())
	}

	where := " WHERE 1 "

	if len(args) == 1 {
		where += " AND id = ? "
		args = append(args, args[0])
	} else if len(args) > 1 {
		where += args[0].(string)
	} else {
		args = append(args, nil)
	}
	fmt.Println(api.tableColumns[tableName])
	if api.tableColumns[tableName].HaveColumn("is_deleted") {
		where += " AND is_deleted = 0"
	}

	api.Query(fmt.Sprintf("SELECT * FROM `%s` %s", tableName, where), args[1:]...).Find(v)
}

// connection 找出两张表之间的关联
/*
	根据belong查询master
	master是要查找的，belong是已知的。
*/
func (api *CRUD) connection(target string, got reflect.Value) ([]interface{}, bool) {
	//"SELECT `master`.* FROM `master` WHERE `belong_id` = ? ", belongID
	//"SELECT `master`.* FROM `master` LEFT JOIN `belong` ON `master`.id = `belong`.master_id WHERE `belong`.id = ?"
	//"SELECT `master`.* FROM `master` LEFT JOIN `belong` ON `master`.belong_id = `belong`.id WHERE `belong`.id = ?"
	//"SELECT `master`.* FROM `master` LEFT JOIN `master_belong` ON `master_belong`.master_id = `master`.id WHERE `master_belong`.belong_id = ?", belongID
	//"SELECT `master`.* FROM `master` LEFT JOIN `belong_master` ON `belong_master`.master_id = `master`.id WHERE `belong_master`.belong_id = ?", belongID
	// 首先实现正常的逻辑，然后再进行所有逻辑的判断。

	ttn := target                      //target table name
	gtn := ToDBName(got.Type().Name()) // got table name

	fmt.Println(ttn, gtn)

	if api.tableColumns[gtn].HaveColumn(ttn + "_id") {
		// got: question_option question_id
		// target: question
		// select * from question where id = question_option.question_id
		//return api.RowSQL(fmt.Sprintf("SELECT `%s`.* FROM `%s` WHERE %s = ?", gtn, gtn, "id"), got.FieldByName(ttn+"_id").Interface())
		return []interface{}{fmt.Sprintf("SELECT `%s`.* FROM `%s` WHERE %s = ?", gtn, gtn, "id"), got.FieldByName(ToStructName(ttn + "_id")).Interface()}, true
	}

	if api.tableColumns[ttn].HaveColumn(gtn + "_id") {
		//got: question
		//target:question_options
		//select * from question_options where question.options.question_id = question.id
		//		return api.RowSQL(fmt.Sprintf("SELECT * FROM `%s` WHERE %s = ?", ttn, gtn+"_id"), got.FieldByName("id").Interface())
		return []interface{}{fmt.Sprintf("SELECT * FROM `%s` WHERE %s = ?", ttn, gtn+"_id"), got.FieldByName("ID").Interface()}, true
	}

	//group_section
	//got: group
	//target: section
	//SELECT section.* FROM section LEFT JOIN group_section ON group_section.section_id = section.id WHERE group_section.group_id = group.id

	ctn := ""
	if api.haveTablename(ttn + "_" + gtn) {
		ctn = ttn + "_" + gtn
	}

	if api.haveTablename(gtn + "_" + ttn) {
		ctn = gtn + "_" + ttn
	}

	if ctn != "" {
		if api.tableColumns[ctn].HaveColumn(gtn+"_id") && api.tableColumns[ctn].HaveColumn(ttn+"_id") {
			//			return api.RowSQL(fmt.Sprintf("SELECT `%s`.* FROM `%s` LEFT JOIN %s ON %s.%s = %s.%s WHERE %s.%s = ?", ttn, ttn, ctn, ctn, ttn+"_id", ttn, "id", ctn, gtn+"_id"),
			//				got.FieldByName("id").Interface())
			return []interface{}{fmt.Sprintf("SELECT `%s`.* FROM `%s` LEFT JOIN %s ON %s.%s = %s.%s WHERE %s.%s = ?", ttn, ttn, ctn, ctn, ttn+"_id", ttn, "id", ctn, gtn+"_id"),
				got.FieldByName("ID").Interface()}, true
		}
	}

	return []interface{}{}, false
}

// FindAll 在需要的时候将自动查询结构体子结构体
func (api *CRUD) FindAll(v interface{}, args ...interface{}) {
	api.Find(v, args...)
	//然后再查找
	/*
		首先实现结构体
		//不处理指针

	*/
	rv := reflect.ValueOf(v).Elem()
	if rv.Kind() == reflect.Struct {
		for i := 0; i < rv.NumField(); i++ {
			if rv.Field(i).Kind() == reflect.Struct {
				//fmt.Println("struct:", rv.Field(i).Type().Name())
				// member feedback
				// dbn := ToDBName(rv.Field(i).Type().Name())
				fmt.Println(ToDBName(rv.Field(i).Type().Name()))
				con, ok := api.connection(ToDBName(rv.Field(i).Type().Name()), rv)
				if ok {
					api.FindAll(rv.Field(i).Addr().Interface(), con...)
				}

				//api.FindAll(rv.Field(i).Addr().Interface())
			}
			if rv.Field(i).Kind() == reflect.Slice {
				con, ok := api.connection(ToDBName(rv.Field(i).Type().Elem().Name()), rv)
				if ok {
					api.FindAll(rv.Field(i).Addr().Interface(), con...)
				}

				//fmt.Println("slice:", rv.Field(i).Type().Elem().Name())
				//api.FindAll(rv.Field(i).Addr().Interface())
			}
		}
	}

	//然后再实现Slice

}
