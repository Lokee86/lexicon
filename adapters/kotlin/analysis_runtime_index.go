package main

import (
	"fmt"
	"strings"
)

type runtimeIndex struct {
	callables       []*runtimeCallable
	callablesByKey  map[string][]*runtimeCallable
	constructors    map[string][]*runtimeCallable
	propertiesByKey map[string][]runtimeProperty
	typesByID       map[string]*runtimeType
	typesByQN       map[string][]*runtimeType
}

type runtimeCallable struct {
	declaration *declaration
	file        *parsedKotlinFile
	id          string
	kind        string
	ownerKind   string
	ownerQN     string
	parameters  map[string][]string
	signature   string
}

type runtimeProperty struct {
	declaration *declaration
	id          string
	ownerQN     string
}

type runtimeType struct {
	declaration *declaration
	file        *parsedKotlinFile
	form        string
	id          string
	kind        string
	ownerQN     string
	qualified   string
}

func newRuntimeIndex() *runtimeIndex {
	return &runtimeIndex{
		callablesByKey:  make(map[string][]*runtimeCallable),
		constructors:    make(map[string][]*runtimeCallable),
		propertiesByKey: make(map[string][]runtimeProperty),
		typesByID:       make(map[string]*runtimeType),
		typesByQN:       make(map[string][]*runtimeType),
	}
}

func (state *analysis) indexRuntimeDeclaration(
	file *parsedKotlinFile,
	declaration *declaration,
	id, kind, ownerQN, ownerKind, qualifiedBase string,
) *runtimeCallable {
	switch kind {
	case "type", "interface":
		target := &runtimeType{
			declaration: declaration, file: file, form: declaration.form, id: id,
			kind: kind, ownerQN: ownerQN, qualified: qualifiedBase,
		}
		state.runtime.typesByID[id] = target
		state.runtime.typesByQN[qualifiedBase] = append(state.runtime.typesByQN[qualifiedBase], target)
	case "field":
		property := runtimeProperty{declaration: declaration, id: id, ownerQN: ownerQN}
		key := runtimeMemberKey(ownerQN, declaration.name)
		state.runtime.propertiesByKey[key] = append(state.runtime.propertiesByKey[key], property)
	case "function", "method", "constructor":
		callable := &runtimeCallable{
			declaration: declaration, file: file, id: id, kind: kind,
			ownerKind: ownerKind, ownerQN: ownerQN, parameters: make(map[string][]string),
			signature: normalizedParameterSignature(declaration.parameters),
		}
		state.runtime.callables = append(state.runtime.callables, callable)
		if kind == "constructor" {
			state.runtime.constructors[runtimeArityKey(ownerQN, len(declaration.parameters))] = append(
				state.runtime.constructors[runtimeArityKey(ownerQN, len(declaration.parameters))], callable,
			)
		} else {
			key := runtimeCallableKey(ownerQN, declaration.name, len(declaration.parameters))
			state.runtime.callablesByKey[key] = append(state.runtime.callablesByKey[key], callable)
		}
		return callable
	}
	return nil
}

func runtimeCallableKey(owner, name string, arity int) string {
	return fmt.Sprintf("%s\x00%s\x00%d", owner, name, arity)
}

func runtimeArityKey(owner string, arity int) string {
	return fmt.Sprintf("%s\x00%d", owner, arity)
}

func runtimeMemberKey(owner, name string) string {
	return owner + "\x00" + name
}

func normalizedParameterSignature(parameters []parameterDecl) string {
	parts := make([]string, len(parameters))
	for index, parameter := range parameters {
		parts[index] = normalizeRuntimeType(parameter.typeName)
	}
	return strings.Join(parts, ",")
}

func normalizeRuntimeType(value string) string {
	return strings.Join(strings.Fields(value), "")
}

func normalizedReceiver(declaration *declaration) string {
	return normalizeRuntimeType(declaration.receiver)
}
