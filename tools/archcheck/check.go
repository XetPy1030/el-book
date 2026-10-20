package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Violation — нарушение правила в строке файла.
type Violation struct {
	File string // путь от корня модуля через «/»
	Line int
	Rule int
	Msg  string
}

func (v Violation) String() string {
	return fmt.Sprintf("%s:%d: правило %d: %s", v.File, v.Line, v.Rule, v.Msg)
}

// skipDirs — папки, которые не проверяются: данные, сборка, сторонний код.
var skipDirs = map[string]bool{".git": true, "backup": true, "build": true, "dist": true, "testdata": true, "node_modules": true}

// koboPkg — единственный пакет, где живёт код под Kobo (правило 4).
const koboPkg = "sdk/device/kobo"

// fsWrites — функции пакета os, которые меняют файловую систему (правило 13).
var fsWrites = map[string]bool{
	"WriteFile": true, "Create": true, "CreateTemp": true, "OpenFile": true,
	"Mkdir": true, "MkdirAll": true, "MkdirTemp": true, "Remove": true, "RemoveAll": true,
	"Rename": true, "Chtimes": true, "Truncate": true, "Symlink": true, "Link": true,
	"Chmod": true, "Chown": true,
}

// Check проверяет модуль в каталоге root.
func Check(root string) ([]Violation, error) {
	module, err := modulePath(root)
	if err != nil {
		return nil, err
	}
	var vs []Violation
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (skipDirs[d.Name()] || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		fileVs, err := checkFile(path, filepath.ToSlash(rel), module)
		vs = append(vs, fileVs...)
		return err
	})
	sort.Slice(vs, func(i, j int) bool {
		if vs[i].File != vs[j].File {
			return vs[i].File < vs[j].File
		}
		return vs[i].Line < vs[j].Line
	})
	return vs, err
}

// modulePath читает имя модуля из go.mod.
func modulePath(root string) (string, error) {
	f, err := os.Open(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if m, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "module "); ok {
			return strings.Trim(strings.TrimSpace(m), `"`), nil
		}
	}
	return "", fmt.Errorf("в %s/go.mod нет строки module", root)
}

// under — лежит ли путь pkg в пакете prefix или под ним.
func under(pkg, prefix string) bool {
	return pkg == prefix || strings.HasPrefix(pkg, prefix+"/")
}

func checkFile(path, rel, module string) ([]Violation, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	pkg := filepath.ToSlash(filepath.Dir(rel)) // папка пакета от корня: «apps/draw»
	isTest := strings.HasSuffix(rel, "_test.go")
	var vs []Violation
	add := func(pos token.Pos, rule int, format string, args ...any) {
		vs = append(vs, Violation{File: rel, Line: fset.Position(pos).Line, Rule: rule, Msg: fmt.Sprintf(format, args...)})
	}

	base := filepath.Base(rel)
	if !under(pkg, koboPkg) && (strings.HasSuffix(base, "_linux_arm.go") || strings.HasSuffix(base, "_linux_arm_test.go")) {
		add(f.Package, 4, "файл под книгу вне %s", koboPkg)
	}

	osName := ""
	for _, imp := range f.Imports {
		p, _ := strconv.Unquote(imp.Path.Value)
		switch {
		case p == "C":
			add(imp.Pos(), 1, `импорт "C" запрещён: сборка без cgo`)
		case (p == "golang.org/x/sys/unix" || p == "unsafe") && !under(pkg, koboPkg):
			add(imp.Pos(), 4, "%s — только в %s", p, koboPkg)
		case p == "os":
			osName = "os"
			if imp.Name != nil {
				osName = imp.Name.Name
			}
		}
		if dep, ok := strings.CutPrefix(p, module+"/"); ok && !isTest {
			if msg, rule := layerViolation(pkg, dep); msg != "" {
				add(imp.Pos(), rule, "%s импортирует %s: %s", pkg, dep, msg)
			}
		}
	}

	if isTest {
		return vs, nil
	}
	checkWrites := (under(pkg, "apps") || under(pkg, "shell") || under(pkg, "sdk")) &&
		!under(pkg, "sdk/storage") && !under(pkg, "sdk/device")
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.GoStmt:
			if under(pkg, "apps") {
				add(n.Pos(), 11, "голый go в приложении: фоновая работа — через ctx.Go")
			}
		case *ast.CallExpr:
			if !checkWrites || osName == "" {
				return true
			}
			sel, ok := n.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if id, ok := sel.X.(*ast.Ident); ok && id.Name == osName && fsWrites[sel.Sel.Name] {
				if sel.Sel.Name == "OpenFile" && readOnly(n, osName) {
					return true
				}
				add(n.Pos(), 13, "os.%s: файлы пишутся только через sdk/storage", sel.Sel.Name)
			}
		}
		return true
	})
	return vs, nil
}

// readOnly — os.OpenFile открывает файл только на чтение (флаг ровно os.O_RDONLY).
func readOnly(call *ast.CallExpr, osName string) bool {
	if len(call.Args) < 2 {
		return false
	}
	sel, ok := call.Args[1].(*ast.SelectorExpr)
	if !ok {
		return false
	}
	id, ok := sel.X.(*ast.Ident)
	return ok && id.Name == osName && sel.Sel.Name == "O_RDONLY"
}

// layerViolation проверяет импорт пакета dep из пакета pkg (пути от корня модуля):
// правила 5–7 и то, что SDK опирается только на себя.
func layerViolation(pkg, dep string) (string, int) {
	deviceImpl := under(dep, "sdk/device/kobo") || under(dep, "sdk/device/emu")
	switch {
	case under(pkg, "apps"):
		if !under(dep, "sdk") {
			return "приложение импортирует только sdk/...", 5
		}
		if deviceImpl {
			return "приложение не знает реализаций устройства", 5
		}
	case under(pkg, "sdk"):
		if !under(dep, "sdk") {
			return "SDK не зависит от оболочки, приложений и сборки", 6
		}
	}
	if deviceImpl && !under(pkg, "cmd") && !under(pkg, "sdk/device") {
		return "реализацию устройства выбирает только cmd/...", 7
	}
	return "", 0
}
