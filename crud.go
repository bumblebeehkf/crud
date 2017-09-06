package crud

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"runtime"
	"strings"

	_ "github.com/go-sql-driver/mysql" //
)

// 变量
var (
	TimeFormat = "2006-01-02 15:04:05" //用于编程经常遇到的时间

	//错误
	ErrExec = errors.New("执行错误")
	ErrArgs = errors.New("参数错误")

	ErrInsertRepeat = errors.New("重复插入")
	ErrSQLSyncPanic = errors.New("SQL语句异常")
	ErrInsertData   = errors.New("插入数据库异常")
	ErrNoUpdateKey  = errors.New("没有更新主键")

	ErrMustNeedAddr   = errors.New("必须为值引用")
	ErrMustNeedSlice  = errors.New("必须为Slice")
	ErrMustNeedID     = errors.New("必须要有ID")
	ErrNotSupportType = errors.New("不支持类型")
)

// Render 用于对接http.HandleFunc直接调用CRUD
type Render func(w http.ResponseWriter, err error, data ...interface{})

// CRUD 本包关键类
type CRUD struct {
	*search

	debug          bool
	tableColumns   map[string]Columns
	dataSourceName string
	db             *sql.DB

	render Render //crud本身不渲染数据，通过其他地方传入一个渲染的函数，然后渲染都是那边处理。
}

// NewCRUD 创建一个新的CRUD链接
func NewCRUD(dataSourceName string, render ...Render) *CRUD {
	db, err := sql.Open("mysql", dataSourceName)
	if err != nil {
		panic(err)
	}
	db.SetMaxIdleConns(20)
	db.SetMaxOpenConns(20)
	crud := &CRUD{
		debug:          false,
		tableColumns:   make(map[string]Columns),
		dataSourceName: dataSourceName,
		db:             db,
		render: func(w http.ResponseWriter, err error, data ...interface{}) {
			if len(render) == 1 {
				if render[0] != nil {
					render[0](w, err, data...)
				}
			}
		},
	}

	for _, tableMap := range crud.Query("SHOW TABLES").RowsMap() {
		for _, table := range tableMap {
			crud.getColumns(table)
		}
	}
	return crud
}

func (api *CRUD) clone() *CRUD {
	c := CRUD{
		debug: api.debug,

		tableColumns:   api.tableColumns,
		dataSourceName: api.dataSourceName,
		db:             api.db,
		render:         api.render,
	}
	if api.search == nil {
		c.search = &search{}
	} else {
		c.search = api.search.clone()
	}
	c.search.db = &c
	return &c
}

/*
	CRUD table
*/

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

/*
	CRUD search
*/

//Where where
func (api *CRUD) Where(query interface{}, args ...interface{}) *CRUD {
	return api.clone().search.Where(fmt.Sprintf("%v", query), args...).db
}

//In In
func (api *CRUD) In(field string, args ...interface{}) *CRUD {
	return api.clone().search.In(field, args...).db
}

//Joins joins
func (api *CRUD) Joins(query string, args ...string) *CRUD {
	return api.clone().search.Joins(query, args...).db
}

//Fields fields
func (api *CRUD) Fields(args ...string) *CRUD {
	return api.clone().search.Fields(args...).db
}

/*
	CRUD colums table
*/

// HaveTable 是否有这张表
func (api *CRUD) HaveTable(tablename string) bool {
	return api.haveTablename(tablename)
}

func (api *CRUD) haveTablename(tableName string) bool {
	_, ok := api.tableColumns[tableName]
	return ok
}

// 获取表中所有列名
func (api *CRUD) getColumns(tableName string) Columns {
	names, ok := api.tableColumns[tableName]
	if ok {
		return names
	}
	rows := api.Query("SELECT COLUMN_NAME,COLUMN_COMMENT,COLUMN_TYPE,DATA_TYPE,IS_NULLABLE FROM information_schema.`COLUMNS` WHERE table_name= ? ", tableName).RowsMap()
	cols := make(map[string]Column)
	for _, v := range rows {
		cols[v["COLUMN_NAME"]] = Column{
			Name:       v["COLUMN_NAME"],
			Comment:    v["COLUMN_COMMENT"],
			ColumnType: v["COLUMN_TYPE"],
			DataType:   v["DATA_TYPE"],
			IsNullAble: v["IS_NULLABLE"] == "YES",
		}
		DBColums[v["COLUMN_NAME"]] = cols[v["COLUMN_NAME"]]
	}
	api.tableColumns[tableName] = cols
	return cols
}

// Table 返回一个Table
func (api *CRUD) Table(tableName string) *Table {
	return &Table{CRUD: api.clone().search.TableName(tableName).db, tableName: tableName}
}

/*
	CRUD debug
*/

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

//Parse 将条件结构转换成sql和参数
func (api *CRUD) Parse() (string, []interface{}) {
	return api.search.Parse()
}

// LogSQL 会将sql语句中的?替换成相应的参数，让DEBUG的时候可以直接复制SQL语句去使用。
func (api *CRUD) LogSQL(sql string, args ...interface{}) {
	if api.debug {
		api.log(getFullSQL(sql, args...))
	}
}

func (api *CRUD) log(args ...interface{}) {
	log.Println(args...)
}

func getFullSQL(sql string, args ...interface{}) string {
	for _, arg := range args {
		sql = strings.Replace(sql, "?", fmt.Sprintf("'%v'", arg), 1)
	}
	return sql
}

// 如果发生了异常就打印调用栈。
func (api *CRUD) stack(err error, sql string, args ...interface{}) {
	buf := make([]byte, 1<<10)
	runtime.Stack(buf, true)
	log.Printf("%s\n%s\n%s\n", err.Error(), getFullSQL(sql, args...), buf)
}

// RowSQL Query alias
func (api *CRUD) RowSQL(sql string, args ...interface{}) *SQLRows {
	return api.Query(sql, args...)
}

/*
	CRUD 查询
*/

// Query 用于底层查询，一般是SELECT语句
func (api *CRUD) Query(sql string, args ...interface{}) *SQLRows {
	db := api.DB()
	api.LogSQL(sql, args...)
	rows, err := db.Query(sql, args...)

	if err != nil {
		api.stack(err, sql, args...)
	}
	return &SQLRows{rows: rows, err: err}
}

// Exec 用于底层执行，一般是INSERT INTO、DELETE、UPDATE。
func (api *CRUD) Exec(sql string, args ...interface{}) sql.Result {
	db := api.DB()
	api.LogSQL(sql, args...)
	ret, err := db.Exec(sql, args...)
	if err != nil {
		api.stack(err, sql, args...)
	}
	return ret
}

// DB 返回一个DB链接，查询后一定要关闭col，而不能关闭*sql.DB。
func (api *CRUD) DB() *sql.DB {
	return api.db
}

//Create 根据相应单个结构体进行创建
func (api *CRUD) Create(obj interface{}) (int64, error) {
	//一定要是地址
	//需要检查Before函数
	//需要按需转换成map(考虑ignore)
	//需要检查After函数
	//TODO 一次性创建整个嵌套结构体
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr {
		return 0, ErrMustNeedAddr
	}
	beforeFunc := v.MethodByName("BeforeCreate")
	afterFunc := v.MethodByName("AfterCreate")
	tableName := getStructDBName(v)

	if beforeFunc.IsValid() {
		beforeFunc.Call(nil)
	}
	m := structToMap(v)
	id, err := api.Table(tableName).Create(m)

	rID := v.Elem().FieldByName("ID")
	if rID.IsValid() {
		rID.SetInt(id)
	}

	if afterFunc.IsValid() {
		afterFunc.Call(nil)
	}
	return id, err
}

//Creates 根据相应多个结构体进行创建
func (api *CRUD) Creates(objs interface{}) ([]int64, error) {
	ids := []int64{}
	v := reflect.ValueOf(objs)
	if v.Kind() != reflect.Ptr {
		return ids, ErrMustNeedAddr
	}
	if v.Elem().Kind() != reflect.Slice {
		return ids, ErrMustNeedSlice
	}

	for i, num := 0, v.Elem().Len(); i < num; i++ {
		id, err := api.Create(v.Elem().Index(i).Addr().Interface())
		if err != nil {
			return ids, err
		}

		ids = append(ids, id)
	}

	return ids, nil
}

//Update Update
func (api *CRUD) Update(obj interface{}) error {
	//根据ID进行Update
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr {
		return ErrMustNeedAddr
	}
	beforeFunc := v.MethodByName("BeforeUpdate")
	afterFunc := v.MethodByName("AfterUpdate")
	if beforeFunc.IsValid() {
		beforeFunc.Call(nil)
	}
	tableName := getStructDBName(v)
	m := structToMap(v)
	err := api.Table(tableName).Update(m)

	if err != nil {
		return err
	}
	if afterFunc.IsValid() {
		afterFunc.Call(nil)
	}
	return nil
}

//Updates Updates
func (api *CRUD) Updates(objs interface{}) error {
	v := reflect.ValueOf(objs)
	if v.Kind() != reflect.Ptr {
		return ErrMustNeedAddr
	}
	if v.Elem().Kind() != reflect.Slice {
		return ErrMustNeedSlice
	}

	for i, num := 0, v.Elem().Len(); i < num; i++ {
		err := api.Update(v.Elem().Index(i).Addr().Interface())
		if err != nil {
			return err
		}
	}
	return nil
}

//Delete Delete
func (api *CRUD) Delete(obj interface{}) (int64, error) {
	v := reflect.ValueOf(obj)
	if v.Kind() != reflect.Ptr {
		return 0, ErrMustNeedAddr
	}
	beforeFunc := v.MethodByName("BeforeDelete")
	afterFunc := v.MethodByName("AfterDelete")
	if beforeFunc.IsValid() {
		beforeFunc.Call(nil)
	}
	id := getStructID(v)
	if id == 0 {
		return 0, ErrMustNeedID
	}
	tableName := getStructDBName(v)

	count, err := api.Table(tableName).Delete(map[string]interface{}{"id": id})
	if afterFunc.IsValid() {
		afterFunc.Call(nil)
	}
	return count, err
}

//Deletes Deletes
func (api *CRUD) Deletes(objs interface{}) (int64, error) {
	var affCount int64
	v := reflect.ValueOf(objs)
	if v.Kind() != reflect.Ptr {
		return 0, ErrMustNeedAddr
	}
	if v.Elem().Kind() != reflect.Slice {
		return 0, ErrMustNeedSlice
	}

	for i, num := 0, v.Elem().Len(); i < num; i++ {
		aff, err := api.Delete(v.Elem().Index(i).Addr().Interface())
		affCount += aff
		if err != nil {
			return affCount, err
		}
	}
	return 0, nil
}

// FormCreate 创建，表单创建。
func (api *CRUD) FormCreate(v interface{}, w http.ResponseWriter, r *http.Request) {
	tableName := getStructDBName(reflect.ValueOf(v))
	m := parseRequest(v, r, C)
	if m == nil || len(m) == 0 {
		api.argsErrorRender(w)
		return
	}
	id, err := api.Table(tableName).Create(m)
	if err != nil {
		api.execErrorRender(w)
		return
	}
	m["id"] = id
	delete(m, "is_deleted")
	api.dataRender(w, m)

	// names := []string{}
	// values := []string{}
	// args := []interface{}{}

	//cols := api.getColums(tableName)
	// if cols.HaveColumn("created_at") {
	// 	m["created_at"] = time.Now().Format(TimeFormat)
	// }
	// if cols.HaveColumn("is_deleted") {1
	// 	m["is_deleted"] = 0
	// }
	// if cols.HaveColumn("updated_at") {
	// 	m["updated_at"] = time.Now().Format(TimeFormat)
	// }

	// for k, v := range m {
	// 	names = append(names, "`"+k+"`")
	// 	values = append(values, "?")
	// 	args = append(args, v)
	// }
	// ret := api.Exec(fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", tableName, strings.Join(names, ","), strings.Join(values, ",")), args...)
	// id, err := ret.LastInsertId()

}

// FormRead 表单查找
/*
	查找
	id = 1
	id = 1  AND hospital_id = 1

	CRUD FormRead -> table Read
*/
func (api *CRUD) FormRead(v interface{}, w http.ResponseWriter, r *http.Request) {
	//	这里传进来的参数一定是要有用的参数，如果是没有用的参数被传进来了，那么会报参数错误，或者显示执行成功数据会乱。
	//	这里处理last_XXX
	//	处理翻页的问题
	//	首先判断这个里面有没有这个字段
	m := parseRequest(v, r, R)

	tableName := getStructDBName(reflect.ValueOf(v))
	data := api.Table(tableName).Reads(m)
	api.dataRender(w, data)
	//	看一下是不是其他表关联查找
	// cols := api.getColums(tableName)
	// ctn := "" //combine table name
	// for k := range m {
	// 	if !cols.HaveColumn(k) {
	// 		if strings.Contains(k, "_id") {
	// 			atn := strings.TrimRight(k, "_id") //another table name
	// 			tmptn := atn + "_" + tableName
	// 			//api.X("检查表" + tmptn)
	// 			if api.haveTablename(tmptn) {
	// 				if api.tableColumns[tmptn].HaveColumn(k) {
	// 					ctn = tmptn
	// 				}
	// 			}
	// 			//api.X("检查表" + tmptn)
	// 			tmptn = tableName + "_" + atn
	// 			if api.haveTablename(tmptn) {
	// 				if api.tableColumns[tmptn].HaveColumn(k) {
	// 					ctn = tmptn
	// 				}
	// 			}
	// 		}
	// 	}
	// }
	// if api.tableColumns[tableName].HaveColumn("is_deleted") {
	// 	m["is_deleted"] = "0"
	// }
	// if ctn == "" {
	// 	//如果没有设置ID，则查找所有的。
	// 	if m == nil || len(m) == 0 {
	// 		data := api.Query(fmt.Sprintf("SELECT * FROM `%s`", tableName)).RawsMapInterface()
	// 		api.dataRender(w, data)
	// 	} else {
	// 		ks, vs := ksvs(m, " = ? ")
	// 		data := api.Query(fmt.Sprintf("SELECT * FROM `%s` WHERE %s", tableName, strings.Join(ks, "AND")), vs...).RawsMapInterface()
	// 		api.dataRender(w, data)
	// 	}
	// } else {
	// 	ks, vs := ksvs(m, " = ? ")
	// 	//SELECT `section`.* FROM `group_section` LEFT JOIN section ON group_section.section_id = section.id WHERE group_id = 1
	// 	data := api.Query(fmt.Sprintf("SELECT `%s`.* FROM `%s` LEFT JOIN `%s` ON `%s`.`%s` = `%s`.`%s` WHERE %s", tableName, ctn, tableName, ctn, tableName+"_id", tableName, "id", strings.Join(ks, "AND")), vs...).RawsMapInterface()
	// 	api.dataRender(w, data)
	// }
}

// FormUpdate 表单更新
func (api *CRUD) FormUpdate(v interface{}, w http.ResponseWriter, r *http.Request) {
	tableName := getStructDBName(reflect.ValueOf(v))
	m := parseRequest(v, r, R)
	if m == nil || len(m) == 0 {
		api.argsErrorRender(w)
		return
	}
	err := api.Table(tableName).Update(m)
	if err != nil {
		api.execErrorRender(w)
		return
	}
	api.ExecSuccessRender(w)
	//	UPDATE task SET name = ? WHERE id = 3;
	//	现在只支持根据ID进行更新
	// id := m["id"]
	// delete(m, "id")
	// if api.tableColumns[tableName].HaveColumn("updated_at") {
	// 	m["updated_at"] = time.Now().Format(TimeFormat)
	// }
	// ks, vs := ksvs(m, " = ? ")
	// vs = append(vs, id)
	// _, err := api.Exec(fmt.Sprintf("UPDATE `%s` SET %s WHERE %s = ?", tableName, strings.Join(ks, ","), "id"), vs...).RowsAffected()
	// if err != nil {
	// 	api.execErrorRender(w)
	// 	return
	// }

}

// FormDelete 表单删除
func (api *CRUD) FormDelete(v interface{}, w http.ResponseWriter, r *http.Request) {
	tableName := getStructDBName(reflect.ValueOf(v))
	m := parseRequest(v, r, R)
	if m == nil || len(m) == 0 {
		api.argsErrorRender(w)
		return
	}
	_, err := api.Table(tableName).Delete(m)
	if err != nil {
		api.execErrorRender(w)
		return
	}
	api.ExecSuccessRender(w)
	// if api.tableColumns[tableName].HaveColumn("is_deleted") {
	// 	if api.tableColumns[tableName].HaveColumn("deleted_at") {
	// 		r.Form["deleted_at"] = []string{time.Now().Format(TimeFormat)}
	// 	}
	// 	r.Form["is_deleted"] = []string{"1"}
	// 	api.Update(v, w, r)
	// 	return
	// }
	//	现在只支持根据ID进行删除
	// ks, vs := ksvs(m, " = ? ")
	// _, err := api.Exec(fmt.Sprintf("DELETE FROM %s WHERE %s ", tableName, strings.Join(ks, "AND")), vs...).RowsAffected()

}

// Find 将查找数据放到结构体里面
// 如果不传条件则是查找所有人
// Read Find Select
func (api *CRUD) Find(obj interface{}, args ...interface{}) error {
	var (
		v = reflect.ValueOf(obj)

		tableName = ""
		elem      = v.Elem()
		where     = " WHERE 1 "

		rawSqlflag = false
	)

	if v.Kind() != reflect.Ptr {
		return ErrMustNeedAddr
	}

	if len(args) > 0 {
		if sql, ok := args[0].(string); ok {
			if strings.Contains(sql, "SELECT") {
				rawSqlflag = true
				err := api.Query(sql, args[1:]...).Find(obj)
				if err != nil {
					return err
				}
			}
		}
	}

	if !rawSqlflag {
		if elem.Kind() == reflect.Slice {
			tableName = getStructDBName(reflect.New(elem.Type().Elem()))
		} else {
			tableName = getStructDBName(elem)
		}

		if len(args) == 1 {
			where += " AND id = ? "
			args = append(args, args[0])
		} else if len(args) > 1 {
			where += "AND " + args[0].(string)
		} else {
			//avoid args[1:]... bounds out of range
			args = append(args, nil)

			//如果没有传参数，那么参数就在结构体本身。（只支持ID,而且是结构体的时候）
			if elem.Kind() == reflect.Struct {
				rID := elem.FieldByName("ID")
				if rID.IsValid() {
					rIDInt64 := rID.Int()
					if rIDInt64 != 0 {
						where += " AND id = ? "
						args = append(args, rIDInt64)
					}
				}
			}
		}

		if api.tableColumns[tableName].HaveColumn("is_deleted") {
			where += " AND is_deleted = 0"
		}

		err := api.Query(fmt.Sprintf("SELECT * FROM `%s` %s", tableName, where), args[1:]...).Find(obj)
		if err != nil {
			return err
		}
	}

	switch elem.Kind() {
	case reflect.Slice:
		for i, num := 0, elem.Len(); i < num; i++ {
			afterFunc := elem.Index(i).Addr().MethodByName("AfterFind")
			if !afterFunc.IsValid() {
				return nil
			}
			afterFunc.Call(nil)
		}
	case reflect.Struct:
		afterFunc := v.MethodByName("AfterFind")
		afterFunc.Call(nil)
	}

	return nil
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

	//fmt.Println(ttn, gtn)

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
func (api *CRUD) FindAll(v interface{}, args ...interface{}) error {
	api.Find(v, args...)
	//首先查找字段，然后再查找结构体和Slice
	/*
		首先实现结构体
		//不处理指针

	*/
	rv := reflect.ValueOf(v).Elem()
	switch rv.Kind() {
	case reflect.Struct:
		api.setStructField(rv)
	case reflect.Slice:
		for i := 0; i < rv.Len(); i++ {
			api.setStructField(rv.Index(i))
		}
	default:
		return ErrNotSupportType
	}
	return nil
}

func (api *CRUD) setStructField(rv reflect.Value) {
	for i := 0; i < rv.NumField(); i++ {
		if rv.Field(i).Kind() == reflect.Struct {
			fmt.Println(ToDBName(rv.Field(i).Type().Name()))
			con, ok := api.connection(ToDBName(rv.Field(i).Type().Name()), rv)
			if ok {
				api.FindAll(rv.Field(i).Addr().Interface(), con...)
			}
		}
		if rv.Field(i).Kind() == reflect.Slice {
			con, ok := api.connection(ToDBName(rv.Field(i).Type().Elem().Name()), rv)
			if ok {
				api.FindAll(rv.Field(i).Addr().Interface(), con...)
			}
		}
	}
}
