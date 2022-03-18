//go:build ignore
// +build ignore

package main

// This program generates *_options_.go files to be used by the bindings calls to API service.
// It can be invoked by running go generate

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"unicode"
	"unicode/utf8"
)

var bodyTmpl = `// Code generated by go generate; DO NOT EDIT.
package {{.PackageName}}

import (
{{range $import := .Imports}}	{{$import}}
{{end}}
)

// Changed returns true if named field has been set
func (o *{{.StructName}}) Changed(fieldName string) bool {
	return util.Changed(o, fieldName)
}

// ToParams formats struct fields to be passed to API service
func (o *{{.StructName}}) ToParams() (url.Values, error) {
	return util.ToParams(o)
}

{{range $field := .Fields}}
// With{{.Name}} set {{if .Comment}}{{.Comment}}{{else}}field {{.Name}} to given value{{end}}
func(o *{{.StructName}}) With{{.Name}}(value {{.Type}}) *{{.StructName}} {
	o.{{.Name}} = {{if not .Composite}}&{{end}}value
	return o
}

// Get{{.Name}} returns value of {{if .Comment}}{{.Comment}}{{else}}field {{.Name}}{{end}}
func(o *{{.StructName}}) Get{{.Name}}() {{.Type}} {
	if o.{{.Name}} == nil {
		var z {{.Type}}
		return z
	}
    return {{if not .Composite}}*{{end}}o.{{.Name}}
}
{{end}}
`

type fieldStruct struct {
	Comment    string
	Composite  bool
	Name       string
	StructName string
	Type       string
}

func main() {
	var (
		closed       bool
		fieldStructs []fieldStruct
	)
	srcFile := os.Getenv("GOFILE")
	inputStructName := os.Args[1]
	b, err := ioutil.ReadFile(srcFile)
	if err != nil {
		panic(err)
	}
	fset := token.NewFileSet() // positions are relative to fset
	f, err := parser.ParseFile(fset, "", b, parser.ParseComments)
	if err != nil {
		panic(err)
	}

	// always add reflect
	imports := []string{"\"reflect\"", "\"github.com/containers/podman/v4/pkg/bindings/internal/util\""}
	for _, imp := range f.Imports {
		imports = append(imports, imp.Path.Value)
	}

	out, err := os.Create(strings.TrimRight(srcFile, ".go") + "_" + strings.Replace(strings.ToLower(inputStructName), "options", "_options", 1) + ".go")
	if err != nil {
		panic(err)
	}
	defer func() {
		if !closed {
			out.Close()
		}
	}()

	body := template.Must(template.New("body").Parse(bodyTmpl))

	ast.Inspect(f, func(n ast.Node) bool {
		ref, refOK := n.(*ast.TypeSpec)
		if !(refOK && ref.Name.Name == inputStructName) {
			return true
		}

		x := ref.Type.(*ast.StructType)
		for _, field := range x.Fields.List {
			var name string
			if len(field.Names) > 0 {
				name = field.Names[0].Name
				if len(name) < 1 {
					panic(errors.New("bad name"))
				}
			}

			var composite bool
			switch field.Type.(type) {
			case *ast.MapType, *ast.StructType, *ast.ArrayType:
				composite = true
			}

			//sub := "*"
			typeExpr := field.Type
			start := typeExpr.Pos() - 1
			end := typeExpr.End() - 1
			fieldType := strings.Replace(string(b[start:end]), "*", "", 1)

			fieldStructs = append(fieldStructs, fieldStruct{
				Comment:    fmtComment(field.Comment.Text()),
				Composite:  composite,
				Name:       name,
				StructName: inputStructName,
				Type:       fieldType,
			})
		} // for

		bodyStruct := struct {
			PackageName string
			Imports     []string
			StructName  string
			Fields      []fieldStruct
		}{
			PackageName: os.Getenv("GOPACKAGE"),
			Imports:     imports,
			StructName:  inputStructName,
			Fields:      fieldStructs,
		}

		// create the body
		if err := body.Execute(out, bodyStruct); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// close out file
		if err := out.Close(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		closed = true

		// go fmt file
		gofmt := exec.Command("go", "fmt", out.Name())
		gofmt.Stderr = os.Stdout
		if err := gofmt.Run(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		// go import file
		goimport := exec.Command("goimports", "-w", out.Name())
		goimport.Stderr = os.Stdout
		if err := goimport.Run(); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		return true
	})
}

func fmtComment(comment string) string {
	r, n := utf8.DecodeRuneInString(comment)
	if r != utf8.RuneError {
		comment = string(unicode.ToLower(r)) + comment[n:]
	}
	comment = strings.TrimSpace(comment)
	return comment
}
