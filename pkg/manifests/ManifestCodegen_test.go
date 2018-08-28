/*
 * Copyright 2018 The microkube authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package manifests

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"path"
	"testing"
)

const (
	// testYAML contains the serviceaccount definition for coreDNS as YAML
	testYAML = `apiVersion: v1
kind: ServiceAccount
metadata:
  name: coredns
  namespace: kube-system`
	// testJSON contains the serviceaccount definition for coreDNS as JSON. This is used to check whether 'testYAML' is
	// converted correctly
	testJSON = "`{\"kind\":\"ServiceAccount\",\"apiVersion\":\"v1\",\"metadata\":{\"name\":\"coredns\",\"namespace\":\"kube-system\",\"creationTimestamp\":null}}`"
)

// TestParse runs the parsing process on a sample YAML and checks the AST of the resulting code file to contain
// 'testJSON'
func TestParse(t *testing.T) {
	srcFile, err := ioutil.TempFile("", "microkube-codegen-test")
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	srcFile.Write([]byte(testYAML))
	srcFile.Close()

	dstDir, err := ioutil.TempDir("", "microkube-codegen-test")
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	uut := ManifestCodegen{
		source: srcFile.Name(),
		dst:    path.Join(dstDir, "UUT.go"),
		name:   "UUT",
		pkg:    "test",
	}
	err = uut.ParseFile()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	err = uut.WriteFiles()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	fileSet := token.NewFileSet()

	astRoot, err := parser.ParseFile(fileSet, path.Join(dstDir, "UUT.go"), nil, 0)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	found := false
	// Check whether the correct variable definition appears in the generated code
	for _, decl := range astRoot.Decls {
		if genericDecl, ok := decl.(*ast.GenDecl); ok {
			if genericDecl.Tok == token.VAR {
				// Found a variable definition
				if len(genericDecl.Specs) == 1 {
					if valSpec, ok := genericDecl.Specs[0].(*ast.ValueSpec); ok {
						if len(valSpec.Names) == 1 && valSpec.Names[0].Name == "kobjSUUTO0" {
							// Found our variable definition
							if len(valSpec.Values) == 1 {
								if basicLiteral, ok := valSpec.Values[0].(*ast.BasicLit); ok {
									if basicLiteral.Value == testJSON {
										found = true
									}
								}
							}
						}
					}
				}
			}
		}
	}

	if !found {
		t.Fatal("Value not found in generated code!")
	}
}
