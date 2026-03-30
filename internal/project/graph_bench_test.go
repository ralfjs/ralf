package project

import (
	"context"
	"fmt"
	"testing"

	"github.com/ralfjs/ralf/internal/parser"
)

func BenchmarkExtractImportsExports(b *testing.B) {
	src := []byte(`
import { useState, useEffect } from 'react';
import axios from 'axios';
import { formatDate, parseDate } from './utils';
import * as config from './config';

export function App() {}
export class Component {}
export const VERSION = '1.0';
export default function main() {}
export { formatDate as format } from './utils';
`)
	p := parser.NewParser(parser.LangJS)
	tree, err := p.Parse(context.Background(), src, nil)
	p.Close()
	if err != nil {
		b.Fatal(err)
	}
	defer tree.Close()

	lineStarts := buildLineIndex(src)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		ExtractImportsExports(tree, src, lineStarts)
	}
}

func BenchmarkNewGraph_100Files(b *testing.B) {
	exports, imports := buildTestData(100, 5)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		NewGraphFromResolved(exports, imports)
	}
}

func BenchmarkNewGraph_1000Files(b *testing.B) {
	exports, imports := buildTestData(1000, 10)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		NewGraphFromResolved(exports, imports)
	}
}

func BenchmarkGraph_HasCycle_1000Files(b *testing.B) {
	exports, imports := buildTestData(1000, 10)
	g := NewGraphFromResolved(exports, imports)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		g.HasCycle()
	}
}

func BenchmarkGraph_ImportedBy(b *testing.B) {
	exports, imports := buildTestData(1000, 10)
	g := NewGraphFromResolved(exports, imports)
	target := "/src/file_50.ts"
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		g.ImportedBy(target)
	}
}

func BenchmarkGraph_CyclicFiles_1000Files(b *testing.B) {
	exports, imports := buildTestData(1000, 10)
	g := NewGraphFromResolved(exports, imports)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		g.CyclicFiles()
	}
}

func BenchmarkResolveSpecifier(b *testing.B) {
	// Bare specifier (fast reject path).
	b.Run("bare", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			ResolveSpecifier("react", "/src/app.ts")
		}
	})
}

func buildTestData(numFiles, importsPerFile int) (exps map[string][]ExportInfo, imps map[string][]ImportInfo) {
	exps = make(map[string][]ExportInfo, numFiles)
	imps = make(map[string][]ImportInfo, numFiles)

	for i := range numFiles {
		path := fmt.Sprintf("/src/file_%d.ts", i)
		exps[path] = []ExportInfo{
			{Name: "default", Kind: "function", Line: 1},
			{Name: fmt.Sprintf("helper_%d", i), Kind: "function", Line: 5},
		}

		fileImports := make([]ImportInfo, 0, importsPerFile)
		for j := range importsPerFile {
			target := fmt.Sprintf("/src/file_%d.ts", (i+j+1)%numFiles)
			fileImports = append(fileImports, ImportInfo{
				Source: target,
				Name:   "default",
				Line:   j + 1,
			})
		}
		imps[path] = fileImports
	}

	return exps, imps
}
