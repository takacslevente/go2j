package main

import (
	"fmt"
	"go/ast"
	"go/token"
	//"go/printer"
	"strings"
)

type Output struct {
	out        *InsertableOut
	tabs       int
	needTabs   bool
	fset       *token.FileSet
	banStmtEnd bool
	blockInfo  BlockInfo
	outSource  *OutSource
	structuralInfo *StructuralInfo
}

type InsertableOut struct {
	buf          *strings.Builder
	insertPoints []*InsertPoint
}

type InsertPoint struct {
	origOut       *Output
	insertOut     *InsertableOut
	pos           int
	postEvalStmt  ast.Stmt
	postEvalStmtFn func(ast.Stmt, *Output)
	postEvalFn func(*Output)
}

type BlockInfo struct {
	VariableTypes    map[string]ast.Expr
	FuncReturnTypes  map[string]ast.Expr
	ReceiverTypeName string
	CurrentFunctionName string
}

type StructuralInfo struct {
	AssignmentLeft bool
	MapAssignment bool
	RangeStmtVars bool
}

func newInsertableOut() *InsertableOut {
	return &InsertableOut{buf: &strings.Builder{}}
}

func (out *Output) getFset() *token.FileSet {
	return out.fset
}

func newResolveTypeOpts() *ResolveTypeOpts {
	return &ResolveTypeOpts{structuralInfo: &StructuralInfo{}}
}

func (out *Output) getPosition() *InsertPoint {
	insertPoint := &InsertPoint{out, newInsertableOut(), len([]byte(out.out.buf.String())), nil, nil, nil}
	out.out.insertPoints = append(out.out.insertPoints, insertPoint)
	return insertPoint
}

func (outPos *InsertPoint) getOut() *Output {
	return &Output{outPos.insertOut, outPos.origOut.tabs, true, outPos.origOut.fset, false, outPos.origOut.blockInfo, outPos.origOut.outSource, outPos.origOut.structuralInfo}
}

func (outPos *InsertPoint) join() {
	out := outPos.origOut.out
	bOut := []byte(out.buf.String())
	
	// join instertation points of inserted part before insering it
	outPos.insertOut.joinInsertPoints()
	res := string(bOut[:outPos.pos]) + outPos.insertOut.buf.String() + string(bOut[outPos.pos:])
	out.buf.Reset()
	out.buf.WriteString(res)
}

func (out *InsertableOut) joinInsertPoints(){
	for i := len(out.insertPoints) - 1; i >= 0; i-- {
		out.insertPoints[i].join()
	}
}

func (outSource *OutSource) joinInsertPoints() {
	outSource.out.joinInsertPoints()
}

func (out *Output) printTabs() {
	if out.needTabs && out.tabs > 0 {
		fmt.Fprint(out.out.buf, strings.Repeat("\t", out.tabs))
		out.needTabs = false
	}
}

func (out *Output) Print(a ...interface{}) {
	out.printTabs()
	fmt.Fprint(out.out.buf, a...)
}

func (out *Output) Println(a ...interface{}) {
	out.printTabs()
	fmt.Fprintln(out.out.buf, a...)
	out.needTabs = true
}

func (out *Output) AddVar(name string, typeExpr ast.Expr) {
	//out.Print("PUT VAR:", name, "-> ")
	//printer.Fprint(out.out.buf, out.fset, typeExpr)
	//out.Println("")
	out.blockInfo.VariableTypes[name] = typeExpr
}

func (out *Output) AddFunc(name string, typeExpr ast.Expr) {
	//out.Println("PUT:", name, "->", typeExpr)
	out.blockInfo.FuncReturnTypes[name] = typeExpr
}

func (out *Output) SetReceiverTypeName(name string) {
	out.blockInfo.ReceiverTypeName = name
}

func (out *Output) SetCurrentFunctionName(name string) {
	out.blockInfo.CurrentFunctionName = name
}

func (out *Output) GetVarType(name string) ast.Expr {
	//out.Println("GET:", name, "->", out.blockInfo.VariableTypes[name])
	return out.blockInfo.VariableTypes[name]
}

func (out *Output) GetReceiverTypeName() string {
	return out.blockInfo.ReceiverTypeName
}

func (out *Output) GetCurrentFunctionName() string {
	return out.blockInfo.CurrentFunctionName
}

func (out *Output) GetFuncType(name string) ast.Expr {
	//out.Println("GET:", name, "->", out.blockInfo.VariableTypes[name])
	return out.blockInfo.FuncReturnTypes[name]
}

func (out *Output) SetOut(insertable *InsertableOut) {
	out.out = insertable
}

func (out *Output) AddTab() *Output {
	return &Output{out.out, out.tabs + 1, true, out.fset, false, out.blockInfo, out.outSource, out.structuralInfo}
}

func (out *Output) NewIndependentOutput() *Output {
	return &Output{newInsertableOut(), 0, true, out.fset, false, out.blockInfo, out.outSource, out.structuralInfo}
}

func (out *Output) BanStmtEnd() *Output {
	return &Output{out.out, out.tabs, out.needTabs, out.fset, true, out.blockInfo, out.outSource, out.structuralInfo}
}

func newOutput(fset *token.FileSet, outSource *OutSource) *Output {
	return &Output{outSource.out, 0, false, fset, false, BlockInfo{map[string]ast.Expr{}, map[string]ast.Expr{}, "", ""}, outSource, &StructuralInfo{}}
}
