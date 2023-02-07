package rpc

import (
	"context"
	"flychain/log"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"unicode"
)

var (
	contextType      = reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType        = reflect.TypeOf((*error)(nil)).Elem()
	subscriptionType = reflect.TypeOf(Subscription{})
	stringType       = reflect.TypeOf("")
)

type serviceRegistry struct {
	mu       sync.Mutex
	services map[string]service
}

// 服务代表一个注册的对象。
type service struct {
	name          string
	callbacks     map[string]*callback // registered handlers
	subscriptions map[string]*callback // available subscriptions/notifications
}

// callback 是在服务器中注册的方法回调
type callback struct {
	fn          reflect.Value  // the function
	rcvr        reflect.Value  // 方法的接收者对象，如果 fn 是方法则设置
	argTypes    []reflect.Type // input argument types
	hasCtx      bool           // method's first argument is a context (not included in argTypes)
	errPos      int            // err return idx, of -1 当方法不能返回错误时
	isSubscribe bool           // true if this is a subscription callback
}

func (r *serviceRegistry) registerName(name string, rcvr interface{}) error {
	rcvrVal := reflect.ValueOf(rcvr)
	if name == "" {
		return fmt.Errorf("no service name for type %s", rcvrVal.Type().String())
	}
	callbacks := suitableCallbacks(rcvrVal)
	if len(callbacks) == 0 {
		return fmt.Errorf("service %T doesn't have any suitable methods/subscriptions to expose", rcvr)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.services == nil {
		r.services = make(map[string]service)
	}
	svc, ok := r.services[name]
	if !ok {
		svc = service{
			name: name,
			callbacks: make(map[string]*callback),
			subscriptions: make(map[string]*callback),
		}
		r.services[name] = svc
	}
	for name, cb := range callbacks {
		if cb.isSubscribe {
			svc.subscriptions[name] = cb
		} else {
			svc.callbacks[name] = cb
		}
	}
	return nil 
}

// callback 返回对应给定 RPC 方法名的回调。
func (r *serviceRegistry) callback(method string) *callback {
	elem := strings.SplitN(method, serviceMethodSeparator, 2)
	if len(elem) != 2 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.services[elem[0]].callbacks[elem[1]]
} 

// 订阅返回给定服务中的订阅回调。
func (r *serviceRegistry) subscription(service, name string) *callback {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.services[service].subscriptions[name]
}

// suitableCallbacks 迭代给定类型的方法。它确定一个方法是否
// 满足 RPC 回调或订阅回调的条件并将其添加到
// 回调集合。有关这些条件的摘要，请参阅服务器文档。
func suitableCallbacks(receiver reflect.Value) map[string]*callback {
	typ := receiver.Type()
	callbacks := make(map[string]*callback)
	for m := 0; m < typ.NumMethod(); m++ {
		method := typ.Method(m)
		if method.PkgPath != "" {
			continue // method not exported
		}
		cb := newCallback(receiver, method.Func)
		if cb == nil {
			continue
		}
		name := formatName(method.Name)
		callbacks[name] = cb
	}
	return callbacks
}

// newCallback 将 fn（一个函数）变成一个回调对象。如果函数返回 nil
// 不适合作为 RPC 回调。
func newCallback(receiver, fn reflect.Value) *callback {
	fntype := fn.Type()
	c := &callback{fn: fn, rcvr: receiver, errPos: -1, isSubscribe: isPubSub(fntype)}
	// 判断参数类型。它们必须都是导出或内置类型。
	c.makeArgTypes()

	// 验证返回类型。该函数最多只能返回一个错误
	// 和/或另一个非错误值。
	outs := make([]reflect.Type, fntype.NumOut())
	for i := 0; i < fntype.NumOut(); i++ {
		outs[i] = fntype.Out(i)
	}
	if len(outs) > 2 {
		return nil
	}
	// 如果返回错误，它必须是最后一个返回值。
	switch {
	case len(outs) == 1 && isErrorType(outs[0]):
		c.errPos = 0
	case len(outs) == 2:
		if isErrorType(outs[0]) || !isErrorType(outs[1]) {
			return nil
		}
		c.errPos = 1
	}
	return c
}

// makeArgTypes 组成 argTypes 列表。
func (c *callback) makeArgTypes() {
	fntype := c.fn.Type()
	// skip receiver and context.Context parameter (if present).
	firstArg := 0
	if c.rcvr.IsValid() {
		firstArg++
	}
	if fntype.NumIn() > firstArg && fntype.In(firstArg) == contextType {
		c.hasCtx = true
		firstArg++
	}
	// Add all remaining parameters
	c.argTypes = make([]reflect.Type, fntype.NumIn()-firstArg)
	for i := firstArg; i < fntype.NumIn(); i++ {
		c.argTypes[i-firstArg] = fntype.In(i)
	}
}

// call 调用回调。
func (c *callback) call(ctx context.Context, method string, args []reflect.Value) (res interface{}, errRes error) {
	// Create the argument slice.
	fullargs := make([]reflect.Value, 0, 2+len(args))
	if c.rcvr.IsValid() {
		fullargs = append(fullargs, c.rcvr)
	}
	if c.hasCtx {
		fullargs = append(fullargs, reflect.ValueOf(ctx))
	}

	// 在运行回调时捕获 panic。
	defer func() {
		if err := recover(); err != nil {
			const size = 64 << 10
			buf := make([]byte, size)
			buf = buf[:runtime.Stack(buf, false)]
			log.Error("RPC method " + method + " crashed: " + fmt.Sprintf("%v\n%s", err, buf))
			errRes = &internalServerError{errcodePanic, "method handler crashed"}
		}
	}()
	// Run the callback
	results := c.fn.Call(fullargs)
	if len(results) == 0 {
		return nil, nil 
	}
	if c.errPos >= 0 && !results[c.errPos].IsNil() {
		// 方法返回了非零错误值。
		err := results[c.errPos].Interface().(error)
		return reflect.Value{}, err
	}
	return results[0].Interface(), nil
}

// Is t context.Context or *context.Context?
func isContextType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t == contextType
}

// t是否满足error接口？
func isErrorType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Implements(errorType)
}

// Is t Subscription or *Subscription?
func isSubscriptionType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t == subscriptionType
}

// isPubSub 测试给定方法是否具有作为第一个参数的 context.Context 和
// 返回对（订阅，错误）。
func isPubSub(methodType reflect.Type) bool {
	//numIn(0) is the receiver type
	if methodType.NumIn() < 2 || methodType.NumOut() != 2 {
		return false
	}
	return isContextType(methodType.In(1)) &&
		isSubscriptionType(methodType.Out(0)) &&
		isErrorType(methodType.Out(1))
}

// formatName 将名称的第一个字符转换为小写。
func formatName(name string) string {
	ret := []rune(name)
	if len(ret) > 0 {
		ret[0] = unicode.ToLower(ret[0])
	}
	return string(ret)
}
