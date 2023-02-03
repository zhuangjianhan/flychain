package main

import (
	"fmt"
	"go/types"
	"reflect"
)

// typeReflectKind 给出代表 typ 的 reflect.Kind。
func typeReflectKind(typ types.Type) reflect.Kind {
	switch typ := typ.(type) {
	case *types.Basic:
		k := typ.Kind()
		if k >= types.Bool && k <= types.Complex128 {
			// Bool..Complex128 的值顺序匹配
			return reflect.Bool + reflect.Kind(k-types.Bool)
		}
		if k == types.String {
			return reflect.String
		}
		if k == types.UnsafePointer {
			return reflect.UnsafePointer
		}
		panic(fmt.Errorf("unhandled BasicKind %v", k))
	case *types.Array:
		return reflect.Array
	case *types.Chan:
		return reflect.Chan
	case *types.Interface:
		return reflect.Interface
	case *types.Map:
		return reflect.Map
	case *types.Pointer:
		return reflect.Pointer
	case *types.Signature:
		return reflect.Func
	case *types.Slice:
		return reflect.Slice
	case *types.Struct:
		return reflect.Struct
	default:
		panic(fmt.Errorf("unhandled type %T", typ))
	}
}

// nonZeroCheck 返回检查“v”是否为“vtyp”类型的非零值的表达式。
func nonZeroCheck(v string, vtyp types.Type, qualify types.Qualifier) string {
	// Resolve type name.
	typ := resolveUnderlying(vtyp)
	switch typ := typ.(type) {
	case *types.Basic:
		k := typ.Kind()
		switch {
		case k == types.Bool:
			return v
		case k >= types.Uint && k <= types.Complex128:
			return fmt.Sprintf("%s != 0", v)
		case k == types.String:
			return fmt.Sprintf(`%s != ""`, v)
		default:
			panic(fmt.Errorf("unhandled BasicKind %v", k))
		}
	case *types.Array, *types.Struct:
		return fmt.Sprintf("%s != (%s{})", v, types.TypeString(vtyp, qualify))
	case *types.Interface, *types.Pointer, *types.Signature:
		return fmt.Sprintf("%s != nil", v)
	case *types.Slice, *types.Map:
		return fmt.Sprintf("len(%s) > 0", v)
	default:
		panic(fmt.Errorf("unhandled type %T", typ))
	}
}

// isBigInt 检查 'typ' 是否为 "math/big".Int。
func isBigInt(typ types.Type) bool {
	named, ok := typ.(*types.Named)
	if !ok {
		return false
	}
	name := named.Obj()
	return name.Pkg().Path() == "math/big" && name.Name() == "Int"
}

// isByte 检查 'typ' 的底层类型是否为 uint8。
func isByte(typ types.Type) bool {
	basic, ok := resolveUnderlying(typ).(*types.Basic)
	return ok && basic.Kind() == types.Uint8
}

func resolveUnderlying(typ types.Type) types.Type {
	for {
		t := typ.Underlying()
		if t == typ {
			return t
		}
		typ = t
	}
}
