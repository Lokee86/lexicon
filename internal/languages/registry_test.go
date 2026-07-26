package languages

import (
	"reflect"
	"testing"
)

func TestDedicatedAdaptersTakePrecedence(t *testing.T) {
	for path, want := range map[string][]string{
		"src/App.svelte":              {"typescript"},
		"src/App.sln":                 {"csharp"},
		"src/App.csproj":              {"csharp"},
		"src/Directory.Build.props":   {"csharp"},
		"src/Directory.Build.targets": {"csharp"},
		"src/global.json":             {"csharp"},
		"src/main.cs":                 {"csharp"},
		"src/main.go":                 {"go"},
		"src/main.py":                 {"python"},
	} {
		if got := ForPath(path); !reflect.DeepEqual(got, want) {
			t.Fatalf("ForPath(%s) = %v, want %v", path, got, want)
		}
	}
}

func TestGenericFallbackUsesExtensionIdentity(t *testing.T) {
	if got, want := ForPath("src/main.c"), []string{"generic-c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ForPath(main.c) = %v, want %v", got, want)
	}
	definition, ok := Lookup("generic-c")
	if !ok || definition.Directory != "generic" || !reflect.DeepEqual(definition.Extensions, []string{".c"}) {
		t.Fatalf("generic definition = %#v, %v", definition, ok)
	}
	if !OwnsSource("generic-c", "src/main.c") || OwnsSource("generic-c", "src/main.cpp") {
		t.Fatal("generic extension ownership is not exact")
	}
}

func TestGenericFallbackExcludesNonSourceFiles(t *testing.T) {
	for _, path := range []string{"README.md", "config.json", "settings.yaml", "data.csv", "image.png"} {
		if got := ForPath(path); got != nil {
			t.Fatalf("ForPath(%s) = %v, want nil", path, got)
		}
	}
	if !OwnsSource("csharp", "src/main.cs") || OwnsSource("generic-cs", "src/main.cs") {
		t.Fatal("C# source ownership is not dedicated")
	}
	if IsGeneric("generic-md") || IsGeneric("generic") {
		t.Fatal("unsupported generic identities must be rejected")
	}
}
