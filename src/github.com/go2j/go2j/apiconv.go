package main

import ()

type JavaApiConv struct {
	method     string
	imports *JavaImport
	argSeparator *ArgSeparator
}

type ArgSeparator struct {
	sep string
}

type JavaTypeConv struct {
	typeName     string
	imports *JavaImport
}

type JavaImport struct {
	typeName     string
	qualifiedName string
}

var JI_DATE = &JavaImport{"Date", "java.util.Date"}
var JI_ARRAY_UTIL = &JavaImport{"ArrayUtil", "org.go2j.util.ArrayUtil"}

var apiConvs = map[string]*JavaApiConv{
	"fmt.Println": &JavaApiConv{method: "System.out.println", argSeparator: &ArgSeparator{sep: " + \" \" + "}},
	"time.Now": &JavaApiConv{method: "new Date", imports: JI_DATE},
	"append": &JavaApiConv{method: "ArrayUtil.append", imports: JI_ARRAY_UTIL},
}

var typeConvs = map[string]*JavaTypeConv{
	"time.Time": &JavaTypeConv{typeName: "Date", imports: JI_DATE},
}
