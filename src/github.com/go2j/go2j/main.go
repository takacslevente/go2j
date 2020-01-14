package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	//"go/printer"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	//"reflect"
	"strings"
)

const APP_VERSION = "0.1"

// The flag package provides a default help printer via -h switch
var versionFlag *bool = flag.Bool("v", false, "Print the version number.")
var printSource *bool = flag.Bool("psrc", false, "Print generated sources.")
var goSrcDir *string = flag.String("gs", "", "Go source path. Required.")
var javaSrcDir *string = flag.String("js", "", "Java source path. Required.")
var goRoot = ""

var oFileSet = &OutFileSet{map[string]*OutSource{}, "", map[string]*OutSource{}, map[string]*Package{}, map[string]bool{}, map[string]string{}}
var oTypes = &OutTypes{map[string]*OutType{}}

type OutFileSet struct {
	set            map[string]*OutSource
	currentPackage string
	classNameSet   map[string]*OutSource
	packageSet     map[string]*Package
	sysPkgs        map[string]bool
	sysImportNames map[string]string
}

type OutSource struct {
	path             string
	name             string
	out              *InsertableOut
	isPackage        bool
	importedPackages map[string]string
	importedClasses  map[string]bool
	importsIP        *InsertPoint
	system           bool
}

type Package struct {
	name            string
	VariableTypes   map[string]ast.Expr
	FuncReturnTypes map[string]ast.Expr
}

type OutTypes struct {
	set map[string]*OutType
}

type OutType struct {
	functionsIP   *InsertPoint
	implementsIP  *InsertPoint
	anyImplements bool
}

type ResolveTypeOpts struct {
	ElementOnly bool
}

func (outTypes *OutTypes) ensure(typeName string) {
	if outTypes.set[typeName] == nil {
		outTypes.set[typeName] = &OutType{}
	}
}

func (outTypes *OutTypes) setFunctionsPos(typeName string, insertPoint *InsertPoint) {
	outTypes.ensure(typeName)
	outTypes.set[typeName].functionsIP = insertPoint
}

func (outTypes *OutTypes) getFunctionsPos(typeName string) *InsertPoint {
	if outTypes.set[typeName] == nil {
		return nil
	}
	return outTypes.set[typeName].functionsIP
}

func (outTypes *OutTypes) setImplementsPos(typeName string, insertPoint *InsertPoint) {
	outTypes.ensure(typeName)
	outTypes.set[typeName].implementsIP = insertPoint
}

func (outTypes *OutTypes) getImplementsPos(typeName string) *InsertPoint {
	if outTypes.set[typeName] == nil {
		return nil
	}
	return outTypes.set[typeName].implementsIP
}

func (outTypes *OutTypes) setAnyImplements(typeName string, anyImplements bool) {
	outTypes.ensure(typeName)
	outTypes.set[typeName].anyImplements = anyImplements
}

func (outTypes *OutTypes) getAnyImplements(typeName string) bool {
	if outTypes.set[typeName] == nil {
		return false
	}
	return outTypes.set[typeName].anyImplements
}

func pathOf(path string) string {
	pathTags := strings.Split(path, "/")
	return strings.Join(pathTags[:len(pathTags)-1], "/")
}

func fileNameOf(path string) string {
	pathTags := strings.Split(path, "/")
	return pathTags[len(pathTags)-1]
}

func newPackage(name string) *Package {
	return &Package{name: name, VariableTypes: map[string]ast.Expr{},
		FuncReturnTypes: map[string]ast.Expr{}}
}

func newOutSource(path string, isPackage bool) *OutSource {
	return &OutSource{path: pathOf(path), name: strings.Title(fileNameOf(path)), isPackage: isPackage, out: newInsertableOut(), importedPackages: map[string]string{}, importedClasses: map[string]bool{}}
}

func (outFileSet *OutFileSet) hasPackage(path string) bool {
	_, has := outFileSet.set[path]
	return has
}

func (outFileSet *OutFileSet) newOutFile(path string, isPackage, isSystem bool) (*OutSource, bool) {
	outSource, has := outFileSet.set[path]
	if has {
		return outSource, !has
	}
	outSource = newOutSource(path, isPackage)
	outSource.system = isSystem
	if !isSystem {
		outFileSet.set[path] = outSource
	}
	return outSource, true
}

func (outSource *OutSource) closeFile() {
	if outSource.isPackage {
		outSource.out.buf.WriteString("}\n")
	}
}

func (outSource *OutSource) addImportName(name, path string) {
	//fmt.Println("addimport:", name, path)
	outSource.importedPackages[name] = path
}

func (outSource *OutSource) addSysImportName(name, path string) {
	outSource.importedClasses[name] = true
	oFileSet.sysImportNames[name] = path
}

func (outSource *OutSource) addImportedClass(name string) {
	outSource.importedClasses[name] = true
	if oFileSet.classNameSet[name] != nil {
		outSource.addImportName(name, oFileSet.classNameSet[name].getFullPackageName())
	}
}

func (outSource *OutSource) writeFile() {
	outBytes := []byte(outSource.out.buf.String())
	ioutil.WriteFile(outSource.getFullFileName(), outBytes, 0644)
}

func (outSource *OutSource) getPackageName() string {
	return convertPath(outSource.path)
}

func (outSource *OutSource) getFullFileName() string {
	return strings.Replace(outSource.getFullPackageName(), ".", "/", -1)
}

func (outSource *OutSource) getFullPackageName() string {
	return outSource.path + "/" + outSource.name
}

func (outSource *OutSource) getFullPath() string {
	return strings.Replace(outSource.path, ".", "/", -1)
}

func (outSource *OutSource) getFullyQualifiedName() string {
	return convertPath(outSource.getFullPackageName())
}

func (outSource *OutSource) getPackage() *Package {
	return oFileSet.packageSet[outSource.getFullPackageName()]
}

func (pkg *Package) AddFunc(name string, typeExpr ast.Expr) {
	pkg.FuncReturnTypes[name] = typeExpr
}

func (pkg *Package) AddVarType(name string, typeExpr ast.Expr) {
	//fmt.Println("add var type:", name)
	pkg.VariableTypes[name] = typeExpr
}

func (pkg *Package) GetVarType(name string) ast.Expr {
	//fmt.Println("get var type:", name)
	return pkg.VariableTypes[name]
}

func (insertPoint *InsertPoint) postEvaluate(outSource *OutSource) {
	for _, subInsertPoint := range insertPoint.insertOut.insertPoints {
		subInsertPoint.postEvaluate(outSource)
	}
	if insertPoint.postEvalExpr != nil {
		out := insertPoint.getOut()
		out.tabs = 0
		typeExpr := resolveType(insertPoint.postEvalExpr, out, insertPoint.resolveOpts)
		if typeExpr != nil {
			out.AddVar(insertPoint.postEvalIdent.Name, typeExpr)
		}
	}
}

func main() {
	flag.Parse() // Scan the arguments list

	if *versionFlag {
		fmt.Println("Go to Java raw cross compiler. Version:", APP_VERSION)
		return
	}

	if *goSrcDir == "" || *javaSrcDir == "" {
		flag.Usage()
		return
	}

	goRoot = os.Getenv("GOROOT")
	if goRoot == "" {
		goRoot = "/usr/local/go"
	}

	srcDir := *goSrcDir
	targetDir := *javaSrcDir
	fileList := FileList(srcDir, "")
	for _, path := range fileList {
		convertSourceFile(path, srcDir+"/")
	}

	// set referenced classes to imported classes
	for _, outSource := range oFileSet.set {
		for importClass, _ := range outSource.importedClasses {
			if oFileSet.classNameSet[importClass] != nil {
				outSource.addImportName(importClass, oFileSet.classNameSet[importClass].getFullPackageName())
			}
		}
	}

	// resolve variable types
	for _, outSource := range oFileSet.set {
		for _, insertPoint := range outSource.out.insertPoints {
			insertPoint.postEvaluate(outSource)
		}

	}

	// add import declarations
	for _, outSource := range oFileSet.set {
		importsOut := outSource.importsIP.getOut()
		for importClass, _ := range outSource.importedClasses {
			if oFileSet.classNameSet[importClass] != nil {
				importsOut.Print("import ", oFileSet.classNameSet[importClass].getFullyQualifiedName())
				importsOut.Println(";")
			} else if oFileSet.sysImportNames[importClass] != "" {
				importsOut.Print("import ", oFileSet.sysImportNames[importClass])
				importsOut.Println(";")
			}
		}
	}

	for _, outSource := range oFileSet.set {
		outSource.joinInsertPoints()
		outSource.closeFile()

	}

	if targetDir != "" {
		os.MkdirAll(targetDir, 0755)
	}
	generateProject(targetDir, fileNameOf(targetDir))
	targetSrcDir := targetDir + "/src"
	generateHelperClasses(targetSrcDir)
	for _, outSource := range oFileSet.set {
		if outSource.system {
			continue
		}
		javaName := outSource.getFullFileName() + ".java"
		if *printSource {
			fmt.Println("----------", outSource.getFullFileName(), "----------")
			fmt.Println(outSource.out.buf.String())
		}
		if targetDir != "" {
			os.MkdirAll(targetSrcDir+"/"+outSource.getFullPath(), 0755)
			if !*printSource {
				fmt.Println("add:", targetSrcDir+"/"+javaName)
			}
			err := ioutil.WriteFile(targetSrcDir+"/"+javaName, []byte(outSource.out.buf.String()), 0644)
			if err != nil {
				fmt.Println("error:", err)
			}
		}
	}

	/*
		for k, v := range oFileSet.classNameSet {
			fmt.Println("respkgs:", k, v.getFullyQualifiedName())
		}

		for _, outSource := range oFileSet.set {
			fmt.Println("---", outSource.getFullFileName())
			for k, v := range outSource.importedPackages {
				fmt.Println("impkgs:", k, v)
				pkg := oFileSet.packageSet[v]
				if pkg != nil {
					for k2, v2 := range pkg.FuncReturnTypes {
						fmt.Println("  frts:", k2, v2)
					}
				}
			}
		}
	*/
}

func parseSystemImport(name string) {
	//fmt.Println("parse sys pkg:", name)
	if oFileSet.sysPkgs[name] {
		return
	}
	oFileSet.sysPkgs[name] = true
	sysSrcDir := goRoot + "/src/" + name
	fileList := FileList(sysSrcDir, "")
	for _, path := range fileList {
		parseSystemSourceFile(path, sysSrcDir+"/")
	}
}

func firstOf(path, sep string) string {
	return strings.Split(path, sep)[0]
}

func convertPath(path string) string {
	return strings.Join(strings.Split(path, "/"), ".")
}

func convertClassName(path string) string {
	pathTokens := strings.Split(path, "/")
	lastToken := pathTokens[len(pathTokens)-1]
	pathTokens[len(pathTokens)-1] = strings.Title(lastToken)
	return strings.TrimPrefix(strings.Join(pathTokens, "."), ".")
}

func getClassPart(path string) string {
	pathTokens := strings.Split(path, ".")
	return pathTokens[len(pathTokens)-1]
}

func convertPackageFileName(path, packageName string) string {
	pathTokens := strings.Split(path, "/")
	pathTokens[len(pathTokens)-1] = packageName
	return strings.Join(pathTokens, "/")
}

func getPackagePath(packageName string) string {
	pathTokens := strings.Split(packageName, "/")
	pathTokens = pathTokens[:len(pathTokens)-1]
	return strings.Join(pathTokens, "/")
}

func parseSystemSourceFile(absPath, trimPrefix string) {
	if strings.HasSuffix(absPath, "_test.go") {
		return
	}
	if !strings.HasSuffix(absPath, ".go") {
		return
	}
	path := strings.TrimPrefix(absPath, trimPrefix)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
		return
	}

	packagePath := convertPackageFileName(path, file.Name.Name)
	// do not   process utilities
	if packagePath == "main" {
		return
	}
	//fmt.Println("sy pkg path:", packagePath)
	//fmt.Println("sy path:", path)
	outSource, isNewPackage := oFileSet.newOutFile(packagePath, true, true)
	out := newOutput(fset, outSource)
	oFileSet.currentPackage = getPackagePath(packagePath)
	if isNewPackage {

		for _, importSpec := range file.Imports {
			convertImportSpec(importSpec, out)
		}
		outSource.importsIP = out.getPosition()
		out.Println()

		oFileSet.set[packagePath] = outSource
		pkg := newPackage(strings.Title(file.Name.Name))
		oFileSet.packageSet[outSource.getFullPackageName()] = pkg
		//fmt.Println("ofspkg2:", outSource.getFullFileName(), strings.Title(file.Name.Name))
		out = out.AddTab()
	}

	for _, decl := range file.Decls {
		convertDecl(decl, out)
	}

}

func convertSourceFile(absPath, trimPrefix string) {
	path := strings.TrimPrefix(absPath, trimPrefix)
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, absPath, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
		return
	}

	packagePath := convertPackageFileName(path, file.Name.Name)
	//fmt.Println("pkg path:", packagePath)
	//fmt.Println("path:", path)
	outSource, isNewPackage := oFileSet.newOutFile(packagePath, true, false)
	out := newOutput(fset, outSource)
	oFileSet.currentPackage = getPackagePath(packagePath)
	if isNewPackage {
		convertPackageHeader(packagePath, out)

		for _, importSpec := range file.Imports {
			convertImportSpec(importSpec, out)
		}
		outSource.importsIP = out.getPosition()
		out.Println()

		convertClassHeader(packagePath, out)
		oFileSet.set[packagePath] = outSource
		pkg := newPackage(strings.Title(file.Name.Name))
		oFileSet.packageSet[outSource.getFullPackageName()] = pkg
		//fmt.Println("ofspkg:", outSource.getFullFileName(), strings.Title(file.Name.Name))

		out = out.AddTab()
	}

	for _, decl := range file.Decls {
		convertDecl(decl, out)
	}

}

func convertImportSpec(importSpec *ast.ImportSpec, out *Output) {
	importSpecPath := strings.Trim(importSpec.Path.Value, "\"")
	ownPackage := firstOf(importSpecPath, "/") == firstOf(out.outSource.getFullPackageName(), "/")
	if ownPackage {
		// own package
		importSpecPath += "/" + strings.Title(fileNameOf(importSpecPath))
	} else {
		parseSystemImport(fileNameOf(importSpecPath))
	}
	importPath := convertClassName(importSpecPath)
	importName := getClassPart(importPath)
	if importSpec.Name != nil {
		importName = importSpec.Name.Name
	}
	// TODO: fix importPath
	if ownPackage {
		out.Print("import ")
		out.Print(importPath)
		out.Println(";")
		out.outSource.addImportName(importName, importSpecPath)
	} else {
		out.outSource.addImportName(importName, "/"+strings.Title(importSpecPath))
	}
}

func convertClassHeader(packagePath string, out *Output) {
	out.Println("public class", out.outSource.name, "{")
}

func convertPackageHeader(packagePath string, out *Output) {
	out.Println("package", out.outSource.getPackageName()+";")
	out.Println("")
}

func convertTypeSpec(typeSpec *ast.TypeSpec, out *Output) {
	// TODO: convert Capital letter type to external file
	// keep track of current file and package
	if typeSpec.Name.IsExported() {
		out = toNewFile(typeSpec.Name.Name, out)
	}
	convertNamedExpr(typeSpec.Type, typeSpec.Name, out)
}

func convertRecv(fieldList *ast.FieldList, out *Output) *Output {
	//printer.Fprint(os.Stdout, out.fset, fieldList)
	if fieldList == nil {
		return out
	}
	field := fieldList.List[0]

	if len(field.Names) == 0 {
		return out
	}
	typeName := resolveTypeName(field.Type, true)
	if typeName != "" {
		outPos := oTypes.getFunctionsPos(typeName)
		if outPos != nil {
			out = outPos.getOut()
			out.SetReceiverTypeName(field.Names[0].Name)
			return out
		}
	}
	return out
}

func convertFuncDecl(funcDecl *ast.FuncDecl, out *Output) {
	//func rcvr name params ret
	out = convertRecv(funcDecl.Recv, out)
	convertExport(funcDecl.Name, out)
	if funcDecl.Recv == nil {
		out.Print("static ")
	}
	convertFuncType(funcDecl.Type, funcDecl.Name.Name, funcDecl.Name.IsExported(), out)
	//out.Println(" {")
	if funcDecl.Body != nil {
		out.SetCurrentFunctionName(funcDecl.Name.Name)
		convertBlockStmt(funcDecl.Body, out)
	}
	//out.Println("}")
}

func convertBlockStmt(blockStmt *ast.BlockStmt, out *Output) {
	convertBlockStmtTab(blockStmt, true, out)
}

func convertBlockStmtTab(blockStmt *ast.BlockStmt, tab bool, out *Output) {
	outList := out
	if blockStmt.Lbrace.IsValid() {
		out.Println(" {")
		if tab {
			outList = out.AddTab()
		}
	}
	for _, stmt := range blockStmt.List {
		convertStmt(stmt, outList)
	}
	if blockStmt.Lbrace.IsValid() {
		out.Println("}")
	}
}

func convertStmt(stmt ast.Stmt, out *Output) {
	//fmt.Println("stmt:", reflect.TypeOf(stmt))
	switch tp := stmt.(type) {
	case *ast.AssignStmt:
		convertAssignStmt(tp, out)
	case *ast.BlockStmt:
		convertBlockStmt(tp, out)
	case *ast.CaseClause:
		convertCaseClause(tp, out)
	case *ast.ExprStmt:
		convertExprStmt(tp, out)
	case *ast.IfStmt:
		convertIfStmt(tp, out)
	case *ast.ForStmt:
		convertForStmt(tp, out)
	case *ast.BranchStmt:
		out.Print(tp.Tok)
		convertStmtEnd(out)
	case *ast.DeclStmt:
		convertDeclStmt(tp, out)
	case *ast.SwitchStmt:
		convertSwitchStmt(tp, out)
	case *ast.EmptyStmt:
		out.Print(";")
	case *ast.IncDecStmt:
		convertIncDecStmt(tp, out)
	case *ast.ReturnStmt:
		convertReturnStmt(tp, out)
	case *ast.RangeStmt:
		convertRangeStmt(tp, out)
	}
}

func convertCaseClause(caseClause *ast.CaseClause, out *Output) {
	for _, expr := range caseClause.List {
		out.Print("case ")
		convertExpr(expr, out)
		out.Println(":")
	}
	for _, stmt := range caseClause.Body {
		convertStmt(stmt, out.AddTab())
	}
	out.AddTab().Println("break;")
}

func convertSwitchStmt(switchStmt *ast.SwitchStmt, out *Output) {
	if switchStmt.Init != nil {
		convertStmt(switchStmt.Init, out)
	}
	out.Print("switch (")
	convertExpr(switchStmt.Tag, out)
	out.Print(")")
	convertBlockStmtTab(switchStmt.Body, false, out)
}

func convertIncDecStmt(incDecStmt *ast.IncDecStmt, out *Output) {
	convertExpr(incDecStmt.X, out)
	convertOp(incDecStmt.Tok, out)
	convertStmtEnd(out)
}

func convertDeclStmt(declStmt *ast.DeclStmt, out *Output) {
	convertDecl(declStmt.Decl, out)
	convertStmtEnd(out)
}

func convertDecl(decl ast.Decl, out *Output) {
	//fmt.Println("decl stmt:", reflect.TypeOf(decl))
	switch tp := decl.(type) {
	case *ast.FuncDecl:
		convertFuncDecl(tp, out)
	case *ast.GenDecl:
		convertGenDecl(tp, out)
	}
}

func convertGenDecl(decl *ast.GenDecl, out *Output) {
	isConst := isConst(decl.Tok)
	isImport := decl.Tok == token.IMPORT
	needParen := decl.Lparen.IsValid() && !isImport

	if needParen {
		out.Print("(")
	}
	for idx, spec := range decl.Specs {
		if idx > 0 && !isImport {
			out.Print(", ")
		}
		convertSpec(spec, isConst, out)
	}
	if needParen {
		out.Print(")")
	}
}

func isConst(tok token.Token) bool {
	return tok.String() == "const"
}

func convertSpec(spec ast.Spec, isConst bool, out *Output) {
	switch tp := spec.(type) {
	case *ast.TypeSpec:
		convertTypeSpec(tp, out)
	case *ast.ValueSpec:
		convertValueSpec(tp, isConst, out)
		// handle import before anything else
		//case *ast.ImportSpec:
		//	convertImportSpec(tp, out)
	}
}

func convertValueSpec(valueSpec *ast.ValueSpec, isConst bool, out *Output) {
	//printer.Fprint(os.Stdout, out.fset, valueSpec)
	for idx, name := range valueSpec.Names {
		convertExport(name, out)
		out.Print("static ")
		convertConst(isConst, out)
		if valueSpec.Type != nil {
			convertType(valueSpec.Type, out, nil)
			out.outSource.getPackage().AddVarType(name.Name, valueSpec.Type)
			out.Print(" ")
		} else {
			if idx < len(valueSpec.Values) {
				resolveType(valueSpec.Values[idx], out, nil)
			}
			//TODO: iota
			out.Print(" ")
		}
		convertIdent(name, out)
		if valueSpec.Values != nil {
			out.Print(" = ")
			if idx < len(valueSpec.Values) {
				convertExpr(valueSpec.Values[idx], out)
			}
			// TODO: iota
		} else if valueSpec.Type != nil {
			switch valueSpec.Type.(type) {
			// automatic initialization of arrays
			case *ast.ArrayType:
				out.Print(" = new ")
				convertType(valueSpec.Type, out, nil)
				out.Print("{}")
			}
		}
		convertStmtEnd(out)
	}
}

func findType(expr ast.Expr, out *Output) ast.Expr {
	switch tp := expr.(type) {
	case *ast.Ident:
		identExpr := out.GetVarType(tp.Name)
		if identExpr != nil {
			return identExpr
		}
	case *ast.CompositeLit:
		return tp.Type
	}
	return expr
}

func resolveTypeName(expr ast.Expr, needTitle bool) string {
	switch tp := expr.(type) {
	case *ast.Ident:
		if needTitle {
			return strings.Title(tp.Name)
		} else {
			return tp.Name
		}
	case *ast.StarExpr:
		return resolveTypeName(tp.X, needTitle)
	case *ast.SelectorExpr:
		return resolveTypeName(tp.X, false) +
			"." +
			resolveTypeName(tp.Sel, needTitle)
	}
	return ""
}

func resolveType(expr ast.Expr, out *Output, opts *ResolveTypeOpts) ast.Expr {
	//out.Println("resolveType tp:", reflect.TypeOf(expr))
	switch tp := expr.(type) {
	case *ast.CompositeLit:
		convertType(tp.Type, out, opts)
	case *ast.BasicLit:
		resolveTypeBasicLit(tp, out)
	case *ast.UnaryExpr:
		resolveType(tp.X, out, opts)
	case *ast.ParenExpr:
		resolveType(tp.X, out, opts)
	case *ast.BinaryExpr:
		resolveType(tp.X, out, opts)
	case *ast.IndexExpr:
		switch subTp := tp.X.(type) {
		case *ast.Ident:
			//fmt.Print("stp:", subTp.Name)
			identExpr := out.GetVarType(subTp.Name)
			switch identTp := identExpr.(type) {
			case *ast.MapType:
				//fmt.Print("map val:", reflect.TypeOf(identTp.Value))
				convertType(identTp.Value, out, opts)
			case *ast.ArrayType:
				convertType(identTp.Elt, out, opts)
			}
		}
	case *ast.Ident:
		identExpr := out.GetVarType(tp.Name)
		if identExpr == nil {
			out.Print("Object")
		} else {
			switch tp := identExpr.(type) {
			case *ast.Ident:
				convertIdent(tp, out)
				return tp
			case *ast.MapType:
				// apply dereferences, if any
			}
			resolveType(identExpr, out, opts)
		}
	case *ast.CallExpr:
		//out.Println("funtp:", reflect.TypeOf(tp.Fun))
		funName := resolveTypeName(tp.Fun, false)
		funNameTokens := strings.Split(funName, ".")
		if len(funNameTokens) > 1 {
			funNamePrefix := funNameTokens[0]
			funNamePostfix := funNameTokens[1]
			if out.GetVarType(funNamePrefix) != nil {
				// variable, discover type, set as package name
				//out.Println("gvt fnp:", funNamePrefix)
				varType := out.GetVarType(funNamePrefix)
				funNamePrefix = resolveTypeName(varType, true)
				//out.Println("gvt:", varType)
			}
			// package name
			//out.Println("fnp:", funNamePrefix)
			if pkgPath := out.outSource.importedPackages[strings.Title(funNamePrefix)]; pkgPath != "" {
				//out.Println("fnp in:", pkgPath)
				pkg := oFileSet.packageSet[pkgPath]
				if pkg != nil {
					//out.Println("fnp funcname:", funNamePostfix)
					rtExpr := pkg.FuncReturnTypes[funNamePostfix]
					if rtExpr != nil {
						qualifiedTypeName := funNamePrefix + "." + resolveTypeName(rtExpr, false)
						if conv, has := typeConvs[qualifiedTypeName]; has {
							out.Print(conv.typeName)
							out.outSource.addSysImportName(conv.imports.typeName, conv.imports.qualifiedName)
						} else {
							convertType(rtExpr, out, opts)
						}
					}
					return rtExpr
				}
			}

		}
		//out.Println("funName:", funName)
		identExpr := out.GetFuncType(funName)
		switch tp := identExpr.(type) {
		case *ast.Ident:
			convertTypeIdent(tp, out)
			return nil
		}
	case *ast.SelectorExpr:
		selName := resolveTypeName(tp.X, false)
		// remove receiver type name from method expressions
		if out.GetReceiverTypeName() == "" || selName != out.GetReceiverTypeName() {
			// TODO
		} else {
			// convert rcvr type field
			pkg := out.outSource.getPackage()
			varType := pkg.GetVarType(tp.Sel.Name)
			convertType(varType, out, opts)
			return varType
		}
	default:
		out.Print("Object")
	}
	return nil
}

func resolveTypeBasicLit(basicLit *ast.BasicLit, out *Output) {
	switch basicLit.Kind {
	case token.INT:
		out.Print("int")
	case token.FLOAT:
		out.Print("float")
	case token.CHAR:
		out.Print("char")
	case token.STRING:
		out.Print("String")
	}
}

func convertConst(isConst bool, out *Output) {
	if isConst {
		out.Print("final ")
	}
}

func convertRangeStmt(rangeStmt *ast.RangeStmt, out *Output) {
	out.Print("for (")
	//out.Print("Object ")

	if rangeStmt.Value == nil || out.outSource.system {
		out.Print("Object ")
	} else {
		out.Print("")
		pos := out.getPosition()
		pos.postEvalExpr = rangeStmt.X
		pos.postEvalIdent = rangeStmt.Value.(*ast.Ident)
		pos.resolveOpts = &ResolveTypeOpts{ElementOnly: true}
		out.Print(" ")
	}

	if rangeStmt.Key != nil {
		out.Print("/* ")
		convertExpr(rangeStmt.Key, out)
		out.Print(" */ ")
	}
	convertExpr(rangeStmt.Value, out)
	out.Print(" : ")
	convertExpr(rangeStmt.X, out)
	out.Print(") ")
	convertBlockStmt(rangeStmt.Body, out)
}

func convertForStmt(forStmt *ast.ForStmt, out *Output) {
	out.Print("for (")
	if forStmt.Init != nil {
		convertStmt(forStmt.Init, out.BanStmtEnd())
	}
	out.Print("; ")
	if forStmt.Cond != nil {
		convertExpr(forStmt.Cond, out)
	}
	out.Print("; ")
	if forStmt.Post != nil {
		convertStmt(forStmt.Post, out.BanStmtEnd())
	}
	out.Print(") ")
	convertBlockStmt(forStmt.Body, out)
}

func convertIfStmt(ifStmt *ast.IfStmt, out *Output) {
	if ifStmt.Init != nil {
		convertStmt(ifStmt.Init, out)
		convertStmtEnd(out)
	}
	out.Print("if (")
	convertExpr(ifStmt.Cond, out)
	out.Print(") ")
	convertBlockStmt(ifStmt.Body, out)
	if ifStmt.Else != nil {
		out.Print(" else ")
		convertStmt(ifStmt.Else, out)
	}
}

func convertExprStmt(exprStmt *ast.ExprStmt, out *Output) {
	convertExpr(exprStmt.X, out)
	convertStmtEnd(out)
}

func convertReturnStmt(returnStmt *ast.ReturnStmt, out *Output) {
	out.Print("return ")
	for idx, expr := range returnStmt.Results {
		if idx > 0 {
			out.Print(" /* ")
		}
		convertExpr(expr, out)
		if idx > 0 {
			out.Print(" */")
		}
		if idx == 0 {
			newOut := out.NewIndependentOutput()
			resolveType(expr, newOut, nil)
			retTypeName := newOut.out.buf.String()
			funName := out.GetCurrentFunctionName()
			funType := out.GetFuncType(funName)
			newOut = out.NewIndependentOutput()
			convertType(funType, newOut, nil)
			funTypeName := newOut.out.buf.String()
			if funTypeName != retTypeName {
				implementsIP := oTypes.getImplementsPos(retTypeName)
				if implementsIP != nil {
					outImplements := implementsIP.getOut()
					outImplements.needTabs = false
					if !oTypes.getAnyImplements(funTypeName) {
						outImplements.Print(" implements ")
					} else {
						outImplements.Print(" ,")
					}
					outImplements.Print(funTypeName)
				}
			}

		}
	}
	convertStmtEnd(out)
}

func convertAssignToken(token token.Token, out *Output) {
	tokenStr := token.String()
	if tokenStr == ":=" {
		out.Print("=")
	} else {
		out.Print(tokenStr)
	}
}

func convertAssignStmt(assignStmt *ast.AssignStmt, out *Output) {
	assignmentToken := assignStmt.Tok.String()
	isDef := assignmentToken == ":="
	if isDef {
		out.Print("")
		//postpone resolving type of assignStmt.Rhs[0]
		pos := out.getPosition()
		pos.postEvalExpr = assignStmt.Rhs[0]
		pos.postEvalIdent = assignStmt.Lhs[0].(*ast.Ident)
		out.Print(" ")
	}
	if len(assignStmt.Lhs) > 1 && len(assignStmt.Rhs) > 1 {
		for idx, expr := range assignStmt.Lhs {
			if idx > 0 {
				out.Print(", ")
			}
			convertExpr(expr, out)
			out.Print(" = ")
			convertExpr(assignStmt.Rhs[idx], out)
		}
	} else {
		for idx, expr := range assignStmt.Lhs {
			if idx > 0 {
				out.Print(" /* ")
			}
			convertExpr(expr, out)
			if idx > 0 {
				out.Print(" */")
			}
		}
		if isDef {
			out.AddVar(assignStmt.Lhs[0].(*ast.Ident).Name, findType(assignStmt.Rhs[0], out))
		}
		out.Print(" ")
		convertAssignToken(assignStmt.Tok, out)
		out.Print(" ")
		convertExpr(assignStmt.Rhs[0], out)
	}
	convertStmtEnd(out)
}

func convertExpr(expr ast.Expr, out *Output) {
	convertNamedExpr(expr, nil, out)
}

func firstSelectorName(expr ast.Expr) string {
	switch tp := expr.(type) {
	case *ast.SelectorExpr:
		return firstSelectorName(tp.X)
	case *ast.Ident:
		return tp.Name
	}
	return ""
}

func convertExprSkipFirstSel(expr ast.Expr, out *Output) {
	switch tp := expr.(type) {
	case *ast.SelectorExpr:
		convertExpr(tp.X, out)
	case *ast.Ident:
		return
	}
}

func convertNamedExpr(expr ast.Expr, ident *ast.Ident, out *Output) {
	//out.Println("expr:", reflect.TypeOf(expr))
	//printer.Fprint(out.out.buf, out.fset, expr)
	switch tp := expr.(type) {
	case *ast.InterfaceType:
		convertInterface(tp, ident, out)
		//printer.Fprint(os.Stdout, fset, tp)
	case *ast.StructType:
		convertStruct(tp, ident, out)
		//printer.Fprint(os.Stdout, out.fset, tp)
	case *ast.Ident:
		convertIdent(tp, out)
	case *ast.BinaryExpr:
		convertExpr(tp.X, out)
		out.Print(" ")
		convertOp(tp.Op, out)
		out.Print(" ")
		convertExpr(tp.Y, out)
	case *ast.BasicLit:
		convertBasicLit(tp, out)
	case *ast.CallExpr:
		funName := resolveTypeName(tp.Fun, false)
		conv := apiConvs[funName]
		if conv != nil {
			out.Print(conv.method)
			if conv.imports != nil {
				out.outSource.addSysImportName(conv.imports.typeName, conv.imports.qualifiedName)
			}
		} else {
			convertExpr(tp.Fun, out)
		}
		out.Print("(")
		for idx, arg := range tp.Args {
			if idx > 0 {
				if conv != nil && conv.argSeparator != nil {
					out.Print(conv.argSeparator.sep)
				} else {
					out.Print(", ")
				}
			}
			convertExpr(arg, out)
		}
		out.Print(")")
	case *ast.CompositeLit:
		out.Print("new ")
		convertType(tp.Type, out, nil)
		out.Print("(")
		for idx, elt := range tp.Elts {
			if idx > 0 {
				out.Print(", ")
			}
			convertExpr(elt, out)
		}
		out.Print(")")
	case *ast.IndexExpr:
		convertExpr(tp.X, out)
		out.Print("[")
		convertExpr(tp.Index, out)
		out.Print("]")
	case *ast.KeyValueExpr:
		convertExpr(tp.Value, out)
	case *ast.TypeAssertExpr:
		out.Print("((")
		convertType(tp.Type, out, nil)
		out.Print(")")
		convertExpr(tp.X, out)
		out.Print(")")
	case *ast.ParenExpr:
		out.Print("(")
		convertExpr(tp.X, out)
		out.Print(")")
	case *ast.SelectorExpr:
		// statikus metódusoknál ne!!
		/*firstSelector := strings.Title(firstSelectorName(tp))
		out.Println("fsn:", firstSelector)
		if out.outSource.importedPackages[firstSelector] {
			convertExprSkipFirstSel(tp.X, out)
			out.Print(tp.Sel.Name)
			return
		} else {*/
		selName := resolveTypeName(tp.X, false)
		// remove receiver type name from method expressions
		if out.GetReceiverTypeName() == "" || selName != out.GetReceiverTypeName() {
			convertExpr(tp.X, out)
		} else {
			out.Print("this")
		}
		//}
		out.Print(".")
		out.Print(tp.Sel.Name)
	case *ast.StarExpr:
		convertExpr(tp.X, out)
	case *ast.UnaryExpr:
		convertUnOp(tp.Op, out)
		convertExpr(tp.X, out)
	}
}

var go2jUnOp = map[string]string{
	"&": "",
	"*": "",
}

var go2jIdent = map[string]string{
	"nil": "null",
}

func convertBasicLit(basicLit *ast.BasicLit, out *Output) {
	out.Print(basicLit.Value)
}

func convertUnOp(op token.Token, out *Output) {
	name := op.String()
	if convName, has := go2jUnOp[name]; has {
		name = convName
	}
	out.Print(name)
}

func convertOp(op token.Token, out *Output) {
	out.Print(op.String())
}

func convertIdent(tp *ast.Ident, out *Output) {
	name := tp.Name
	if convName, has := go2jIdent[name]; has {
		name = convName
	}

	if out.GetVarType(name) == nil && out.outSource.importedPackages[strings.Title(name)] != "" {
		name = strings.Title(name)
	}
	out.Print(name)
}

func convertStruct(tp *ast.StructType, ident *ast.Ident, out *Output) {
	out.Print("public ")
	if !ident.IsExported() {
		// inner class in package
		out.Print("static ")
	}
	out.Print("class ")
	name := strings.Title(ident.Name)
	out.Print(name)

	firstIdent := true
	secondIdent := false
	anyImplements := false
	for _, field := range tp.Fields.List {
		switch tp := field.Type.(type) {
		case *ast.Ident:
			if len(field.Names) == 0 {
				if firstIdent {
					out.Print(" extends ")
					firstIdent = false
					secondIdent = true
				} else if secondIdent {
					out.Print(" implements ")
					secondIdent = false
					anyImplements = true
				} else {
					out.Print(", ")
				}
				convertType(tp, out, nil)
			} else {
				break
			}
		}
	}

	oTypes.setImplementsPos(name, out.getPosition())
	oTypes.setAnyImplements(name, anyImplements)

	out.Println(" {")
	for _, field := range tp.Fields.List {
		convertField(field, out.AddTab())
	}

	oTypes.setFunctionsPos(name, out.AddTab().getPosition())

	out.Println("}")
}

func convertField(field *ast.Field, out *Output) {
	if len(field.Names) > 0 {
		convertExport(field.Names[0], out)
		convertType(field.Type, out, nil)
		out.outSource.getPackage().AddVarType(field.Names[0].Name, field.Type)
		out.Print(" ")
		out.Print(field.Names[0].Name)
		if field.Type != nil {
			switch field.Type.(type) {
			case *ast.ArrayType:
				out.Print(" = new ")
				convertType(field.Type, out, nil)
				out.Print("{}")
			}
		}
		convertStmtEnd(out)
	}
}

func convertExport(ident *ast.Ident, out *Output) {
	if ident.IsExported() || ident.Name == "main" {
		out.Print("public ")
	} else {
		out.Print("protected ")
	}
}

func toNewFile(name string, origOut *Output) *Output {
	//fmt.Println("new type pkg:", oFileSet.currentPackage)
	//fmt.Println("new type name:", name)
	path := oFileSet.currentPackage + "/" + name
	outSource, isNew := oFileSet.newOutFile(path, false, origOut.outSource.system)
	out := newOutput(origOut.getFset(), outSource)
	if isNew {
		convertPackageHeader(oFileSet.currentPackage, out)
		oFileSet.classNameSet[name] = outSource
		outSource.importsIP = out.getPosition()
		pkg := newPackage(name)
		oFileSet.packageSet[outSource.getFullPackageName()] = pkg
		//fmt.Println("ofspkg1:", outSource.getFullFileName(), name)
	}
	return out
}

func convertInterface(tp *ast.InterfaceType, ident *ast.Ident, out *Output) {
	if ident.IsExported() {
		out = toNewFile(ident.Name, out)
	}
	out.Print("public interface ")
	out.Print(ident.Name)

	firstIdent := true
	for _, meth := range tp.Methods.List {
		switch tp := meth.Type.(type) {
		case *ast.Ident:
			if firstIdent {
				out.Print(" extends ")
				firstIdent = false
			} else {
				out.Print(", ")
			}
			out.Print(tp.Name)
		}
	}

	out.Println(" {")
	for _, meth := range tp.Methods.List {
		switch tp := meth.Type.(type) {
		case *ast.FuncType:
			funcName := ""
			if len(meth.Names) > 0 {
				funcName = meth.Names[0].Name
			}
			convertFuncType(tp, funcName, true, out.AddTab())
			convertStmtEnd(out)
		}
	}
	out.Println("}")
}

func convertFuncType(tp *ast.FuncType, funcName string, isExported bool, out *Output) {
	if tp.Results == nil {
		out.Print("void ")
	} else {
		for idx, field := range tp.Results.List {
			convertType(field.Type, out, nil)
			out.Print(" ")
			if idx == 0 {
				if isExported {
					//out.Println("expfn pgknm:", out.outSource.getPackage().name)
					out.outSource.getPackage().AddFunc(funcName, field.Type)
					//out.Println("expfn pgknmlen:", len(out.outSource.getPackage().FuncReturnTypes))
					//out.Println("expfn pgknmlen2:", len(oFileSet.packageSet[out.outSource.getFullFileName()].FuncReturnTypes))
				}
				out.AddFunc(funcName, field.Type)
			}
		}
	}
	out.Print(funcName)
	out.Print("(")
	idx := 0
	for _, field := range tp.Params.List {
		for _, name := range field.Names {
			out.AddVar(name.Name, field.Type)

			if idx > 0 {
				out.Print(", ")
			}
			convertType(field.Type, out, nil)
			out.Print(" ")
			out.Print(name.Name)
			idx++
		}
	}
	if funcName == "main" && len(tp.Params.List) == 0 {
		out.Print("String[] args")
	}
	out.Print(")")
}

func convertStmtEnd(out *Output) {
	if out.banStmtEnd {
		return
	}
	out.Println(";")
}

func convertTypeSelectorExpr(selectorExpr *ast.SelectorExpr, out *Output) {
	firstSelector := strings.Title(firstSelectorName(selectorExpr))
	if out.outSource.importedPackages[firstSelector] != "" {
		convertExprSkipFirstSel(selectorExpr.X, out)
	} else {
		convertType(selectorExpr.X, out, nil)
		out.Print(".")
	}
	out.Print(selectorExpr.Sel.Name)
}

func convertType(fieldType ast.Expr, out *Output, opts *ResolveTypeOpts) {
	switch tp := fieldType.(type) {
	case *ast.Ident:
		convertTypeIdent(tp, out)
	case *ast.FuncType:
		convertFuncType(tp, "", false, out)
	case *ast.ArrayType:
		if opts != nil && opts.ElementOnly {
			opts.ElementOnly = false
			convertType(tp.Elt, out, opts)
		} else {
			convertType(tp.Elt, out, opts)
			out.Print("[]")
		}
	case *ast.MapType:
		out.Print("Map<")
		convertType(tp.Key, out, opts)
		out.Print(",")
		convertType(tp.Value, out, opts)
		out.Print(">")
	case *ast.StarExpr:
		convertType(tp.X, out, opts)
	case *ast.Ellipsis:
		convertType(tp.Elt, out, opts)
		out.Print("...")
	case *ast.SelectorExpr:
		firstSelector := strings.Title(firstSelectorName(tp))
		//out.Println("fsn:", firstSelector)
		if out.outSource.importedPackages[firstSelector] != "" {
			convertExprSkipFirstSel(tp.X, out)
			out.Print(tp.Sel.Name)
			out.outSource.addImportedClass(tp.Sel.Name)
			//out.outSource.importedClasses[tp.Sel.Name] = true
			return
		} else {
			qualifiedTypeName := resolveTypeName(tp, false)
			if conv, has := typeConvs[qualifiedTypeName]; has {
				out.Print(conv.typeName)
				out.outSource.addSysImportName(conv.imports.typeName, conv.imports.qualifiedName)
			} else {
				convertType(tp.X, out, opts)
				out.Print(".")
				out.Print(tp.Sel.Name)
			}
		}
	case *ast.InterfaceType:
		out.Print("Object")
	}
}

var go2jType = map[string]string{
	"string": "String",
	"bool":   "boolean",
	"byte":   "byte",
	"int":    "int",
	"int64":  "long",
}

func convertTypeIdent(tp *ast.Ident, out *Output) {
	name := strings.Title(tp.Name)
	if convName, has := go2jType[tp.Name]; has {
		name = convName
	} else {
		out.outSource.addImportedClass(name)
	}
	out.Print(name)
}
