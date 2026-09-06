package lang

import (
	"testing"
)

func TestRustFrontend_SelfParse(t *testing.T) {
	f := NewRustFrontend()
	src := []byte(`use std::collections::HashMap;
use std::fmt::{self, Display};

const MAX_SIZE: usize = 100;
static COUNTER: i32 = 0;

struct Config {
    name: String,
    port: u16,
}

trait Greet {
    fn greet(&self) -> String;
}

impl Greet for Config {
    fn greet(&self) -> String {
        format!("hello {}", self.name)
    }
}

impl Config {
    fn new(name: String, port: u16) -> Config {
        Config { name, port }
    }
}

fn main() {
    let cfg = Config::new("test".to_string(), 8080);
    println!("{}", cfg.greet());
}
`)

	fa, err := f.Analyze("main.rs", src)
	if err != nil {
		t.Fatal(err)
	}

	// HashMap, fmt (self-import), Display
	if len(fa.Imports) != 3 {
		t.Errorf("expected 3 imports (HashMap, fmt, Display), got %d", len(fa.Imports))
		for _, imp := range fa.Imports {
			t.Logf("  import %s from %s (%s)", imp.Name, imp.SourceModule, imp.Kind)
		}
	}

	var funcs, methods, types, consts int
	for _, d := range fa.Definitions {
		switch d.Kind {
		case "function":
			funcs++
		case "method":
			methods++
		case "type":
			types++
		case "constant":
			consts++
		}
	}

	if funcs != 1 {
		t.Errorf("expected 1 function (main), got %d", funcs)
	}
	// Greet.greet (trait signature), Config.greet (impl), Config.new (impl)
	if methods != 3 {
		t.Errorf("expected 3 methods, got %d", methods)
	}
	if types != 2 {
		t.Errorf("expected 2 types (Config, Greet), got %d", types)
	}
	if consts != 2 {
		t.Errorf("expected 2 constants (MAX_SIZE, COUNTER), got %d", consts)
	}

	var sawImplGreet bool
	for _, d := range fa.Definitions {
		if d.Name == "greet" && d.Kind == "method" && d.QualifiedName == "Config.greet" {
			sawImplGreet = true
		}
	}
	if !sawImplGreet {
		t.Error("expected an impl method qualified as Config.greet")
	}

	for _, d := range fa.Definitions {
		t.Logf("  %s (%s) %s line %d", d.Name, d.Kind, d.QualifiedName, d.Line)
	}
}

func TestRustFrontend_ExternBlockAndMacro(t *testing.T) {
	f := NewRustFrontend()
	src := []byte(`extern "C" {
    fn c_helper(x: i32) -> i32;
    static mut G_COUNTER: i32;
}

macro_rules! my_macro {
    () => {};
}
`)

	fa, err := f.Analyze("ffi.rs", src)
	if err != nil {
		t.Fatal(err)
	}

	externByName := make(map[string]Import)
	for _, imp := range fa.Imports {
		if imp.Kind == "extern_c" {
			externByName[imp.Name] = imp
		}
	}
	if _, ok := externByName["c_helper"]; !ok {
		t.Errorf("expected extern_c import c_helper, got %+v", fa.Imports)
	}
	if _, ok := externByName["G_COUNTER"]; !ok {
		t.Errorf("expected extern_c import G_COUNTER, got %+v", fa.Imports)
	}

	found := false
	for _, d := range fa.Definitions {
		if d.Name == "my_macro" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected macro_rules! definition my_macro, got %+v", fa.Definitions)
	}
}

func TestRustFrontend_ParamCounting(t *testing.T) {
	f := NewRustFrontend()
	src := []byte(`fn grouped(a: i32, b: i32) {}

fn no_params() {}

extern "C" fn variadic_c(fmt: *const i8, ...) {}

struct Foo;

impl Foo {
    fn method_self(&self, x: i32) -> i32 { x }
    fn method_mut_self(&mut self, x: i32) {}
    fn assoc_fn(x: i32, y: i32) -> i32 { x + y }
}
`)

	fa, err := f.Analyze("params.rs", src)
	if err != nil {
		t.Fatal(err)
	}

	paramsByName := make(map[string]int)
	for _, d := range fa.Definitions {
		paramsByName[d.Name] = d.ParamCount
	}

	tests := []struct {
		name string
		want int
	}{
		{"grouped", 2},
		{"no_params", 0},
		{"variadic_c", 2},
		{"method_self", 1},
		{"method_mut_self", 1},
		{"assoc_fn", 2},
	}
	for _, tt := range tests {
		got, ok := paramsByName[tt.name]
		if !ok {
			t.Errorf("function %s not found", tt.name)
			continue
		}
		if got != tt.want {
			t.Errorf("%s: expected %d params, got %d", tt.name, tt.want, got)
		}
	}
}

func TestRustFrontend_UseImports(t *testing.T) {
	f := NewRustFrontend()
	src := []byte(`use std::io::{self, Read, Write as W};
use std::collections::*;
use crate::config::Settings;

fn main() {}
`)

	fa, err := f.Analyze("imports.rs", src)
	if err != nil {
		t.Fatal(err)
	}

	byName := make(map[string]Import)
	for _, imp := range fa.Imports {
		byName[imp.Name] = imp
	}

	if imp, ok := byName["io"]; !ok || imp.SourceModule != "std::io" {
		t.Errorf("expected self-import io from std::io, got %+v (ok=%v)", imp, ok)
	}
	if imp, ok := byName["Read"]; !ok || imp.SourceModule != "std::io::Read" || imp.Kind != "type" {
		t.Errorf("expected Read (type) from std::io::Read, got %+v (ok=%v)", imp, ok)
	}
	if imp, ok := byName["W"]; !ok || imp.SourceModule != "std::io::Write" {
		t.Errorf("expected aliased W from std::io::Write, got %+v (ok=%v)", imp, ok)
	}
	if imp, ok := byName["*"]; !ok || imp.SourceModule != "std::collections" {
		t.Errorf("expected wildcard from std::collections, got %+v (ok=%v)", imp, ok)
	}
	if imp, ok := byName["Settings"]; !ok || imp.SourceModule != "crate::config::Settings" {
		t.Errorf("expected Settings from crate::config::Settings, got %+v (ok=%v)", imp, ok)
	}
}

func TestRustFrontend_ForeignABI(t *testing.T) {
	f := NewRustFrontend()
	src := []byte(`#[no_mangle]
pub extern "C" fn rust_entry(x: i32) -> i32 {
    x
}

fn normal_fn(x: i32) -> i32 {
    x
}

struct Foo;

impl Foo {
    extern "C" fn ffi_method(x: i32) -> i32 {
        x
    }
}
`)

	fa, err := f.Analyze("abi.rs", src)
	if err != nil {
		t.Fatal(err)
	}

	abiByName := make(map[string]string)
	for _, d := range fa.Definitions {
		abiByName[d.Name] = d.ForeignABI
	}

	if abiByName["rust_entry"] != "C" {
		t.Errorf("expected rust_entry ForeignABI=C, got %q", abiByName["rust_entry"])
	}
	if abiByName["ffi_method"] != "C" {
		t.Errorf("expected ffi_method ForeignABI=C, got %q", abiByName["ffi_method"])
	}
	if abiByName["normal_fn"] != "" {
		t.Errorf("expected normal_fn to have no ForeignABI, got %q", abiByName["normal_fn"])
	}
}

func TestRustFrontend_ImportKindHardening(t *testing.T) {
	f := NewRustFrontend()
	// `helper` is snake_case (casing heuristic would call it "function"),
	// but it's locally defined as a struct (a "type") via a self-import --
	// the hardened classification should trust the local definition.
	src := []byte(`use self::helper;

struct helper;

fn main() {
    let _ = helper;
}
`)

	fa, err := f.Analyze("harden.rs", src)
	if err != nil {
		t.Fatal(err)
	}

	var found bool
	for _, imp := range fa.Imports {
		if imp.Name == "helper" {
			found = true
			if imp.Kind != "type" {
				t.Errorf("expected hardened Kind=type for local struct helper, got %q", imp.Kind)
			}
		}
	}
	if !found {
		t.Error("expected an import named helper")
	}
}

func TestRustFrontend_MacroReferenceCount(t *testing.T) {
	f := NewRustFrontend()
	src := []byte(`struct Handler;

struct Orphan;

macro_rules! register {
    ($t:ty) => {
        impl $t {
            fn registered() -> bool { true }
        }
    };
}

register!(Handler);

fn main() {}
`)

	fa, err := f.Analyze("macro.rs", src)
	if err != nil {
		t.Fatal(err)
	}

	countByName := make(map[string]int)
	for _, d := range fa.Definitions {
		countByName[d.Name] = d.MacroReferenceCount
	}

	if countByName["Handler"] < 1 {
		t.Errorf("expected Handler to have at least 1 macro reference, got %d", countByName["Handler"])
	}
	if countByName["Orphan"] != 0 {
		t.Errorf("expected Orphan to have 0 macro references, got %d", countByName["Orphan"])
	}
}

func TestRustFrontend_ModuleKind(t *testing.T) {
	f := NewRustFrontend()
	src := []byte(`mod helpers;

use self::helpers;
`)

	fa, err := f.Analyze("modkind.rs", src)
	if err != nil {
		t.Fatal(err)
	}

	var defFound bool
	for _, def := range fa.Definitions {
		if def.Name == "helpers" {
			defFound = true
			if def.Kind != "module" {
				t.Errorf("expected mod_item Kind=module, got %q", def.Kind)
			}
		}
	}
	if !defFound {
		t.Error("expected a definition named helpers")
	}

	var impFound bool
	for _, imp := range fa.Imports {
		if imp.Name == "helpers" {
			impFound = true
			if imp.Kind != "module" {
				t.Errorf("expected hardened import Kind=module for local mod helpers, got %q", imp.Kind)
			}
		}
	}
	if !impFound {
		t.Error("expected an import named helpers")
	}
}

func TestRustFrontend_MacroReferenceAmbiguousNameSkipped(t *testing.T) {
	f := NewRustFrontend()
	src := []byte(`struct A;
struct B;

impl A {
    fn new() -> Self { A }
}

impl B {
    fn new() -> Self { B }
}

macro_rules! noop {
    ($t:tt) => {};
}

noop!(new);
`)

	fa, err := f.Analyze("ambiguous_macro.rs", src)
	if err != nil {
		t.Fatal(err)
	}

	for _, d := range fa.Definitions {
		if d.Name == "new" && d.MacroReferenceCount != 0 {
			t.Errorf("expected ambiguous name %q (qualified %q) to be skipped, got MacroReferenceCount=%d",
				d.Name, d.QualifiedName, d.MacroReferenceCount)
		}
	}
}
