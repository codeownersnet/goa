package functiontool

import (
	"context"
	"fmt"
	"reflect"

	"github.com/codeownersnet/goa/content"
	"github.com/codeownersnet/goa/schema"
	"github.com/codeownersnet/goa/tool"
)

type Config struct {
	Name          string
	Description   string
	InputSchema   *schema.Schema
	IsLongRunning bool
}

type Func[TArgs, TResults any] func(context.Context, TArgs) (TResults, error)

func New[TArgs, TResults any](cfg Config, handler Func[TArgs, TResults]) (tool.Tool, error) {
	var zeroArgs TArgs
	argsType := reflect.TypeOf(zeroArgs)
	for argsType != nil && argsType.Kind() == reflect.Pointer {
		argsType = argsType.Elem()
	}
	if argsType == nil || (argsType.Kind() != reflect.Struct && argsType.Kind() != reflect.Map) {
		return nil, fmt.Errorf("input must be a struct or a map or a pointer to those types, but received: %v", argsType)
	}

	ischema := cfg.InputSchema
	if ischema == nil {
		ischema = inferSchema(argsType)
	}

	return &functionTool[TArgs, TResults]{
		cfg:     cfg,
		handler: handler,
		schema:  ischema,
	}, nil
}

type functionTool[TArgs, TResults any] struct {
	cfg     Config
	handler Func[TArgs, TResults]
	schema  *schema.Schema
}

func (f *functionTool[TArgs, TResults]) Name() string {
	return f.cfg.Name
}

func (f *functionTool[TArgs, TResults]) Description() string {
	return f.cfg.Description
}

func (f *functionTool[TArgs, TResults]) Process(ctx context.Context, args map[string]any) (map[string]any, error) {
	input, err := convertMapTo[TArgs](args)
	if err != nil {
		return nil, fmt.Errorf("failed to convert arguments: %w", err)
	}

	output, err := f.handler(ctx, input)
	if err != nil {
		return nil, err
	}

	result, err := convertToMap(output)
	if err != nil {
		wrapped := map[string]any{"result": output}
		return wrapped, nil
	}
	return result, nil
}

func inferSchema(t reflect.Type) *schema.Schema {
	if t == nil {
		return nil
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	switch t.Kind() {
	case reflect.Struct:
		props := make(map[string]*schema.Schema)
		var required []string
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := field.Name
			omitempty := false
			if tag := field.Tag.Get("json"); tag != "" {
				name = tag
				if idx := len(tag); idx > 0 {
					for j, ch := range tag {
						if ch == ',' {
							name = tag[:j]
							rest := tag[j+1:]
							if rest == "omitempty" || len(rest) > 8 && rest[:9] == "omitempty," {
								omitempty = true
							}
							break
						}
					}
				}
			}
			props[name] = inferFieldSchema(field.Type)
			if !omitempty {
				required = append(required, name)
			}
		}
		return schema.Object(props, required...)
	case reflect.Map:
		return &schema.Schema{Type: "object"}
	case reflect.Slice, reflect.Array:
		return &schema.Schema{Type: "array"}
	case reflect.String:
		return &schema.Schema{Type: "string"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return &schema.Schema{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		return &schema.Schema{Type: "number"}
	case reflect.Bool:
		return &schema.Schema{Type: "boolean"}
	default:
		return &schema.Schema{Type: "string"}
	}
}

func inferFieldSchema(t reflect.Type) *schema.Schema {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return inferSchema(t)
}

func convertValue(val any, targetType reflect.Type) (reflect.Value, error) {
	valType := reflect.TypeOf(val)
	if valType == nil {
		return reflect.Zero(targetType), nil
	}
	if valType.AssignableTo(targetType) {
		return reflect.ValueOf(val), nil
	}
	if targetType.Kind() == reflect.Slice && valType.Kind() == reflect.Slice {
		src := reflect.ValueOf(val)
		slice := reflect.MakeSlice(targetType, src.Len(), src.Len())
		elemType := targetType.Elem()
		for i := 0; i < src.Len(); i++ {
			elem := src.Index(i).Interface()
			converted, err := convertValue(elem, elemType)
			if err != nil {
				return reflect.Value{}, fmt.Errorf("slice element %d: %w", i, err)
			}
			slice.Index(i).Set(converted)
		}
		return slice, nil
	}
	if targetType.Kind() == reflect.Slice && valType.Kind() != reflect.Slice {
		return reflect.Value{}, fmt.Errorf("cannot assign %v to %v", valType, targetType)
	}
	if valType.ConvertibleTo(targetType) {
		return reflect.ValueOf(val).Convert(targetType), nil
	}
	return reflect.Value{}, fmt.Errorf("cannot assign %v to %v", valType, targetType)
}

func convertMapTo[T any](m map[string]any) (T, error) {
	var result T
	resultType := reflect.TypeOf(result)
	if resultType == nil {
		return result, nil
	}
	if resultType.Kind() == reflect.Map {
		rv := reflect.ValueOf(&result).Elem()
		if rv.IsNil() {
			rv.Set(reflect.MakeMap(resultType))
		}
		for k, v := range m {
			rv.SetMapIndex(reflect.ValueOf(k), reflect.ValueOf(v))
		}
		return result, nil
	}
	if resultType.Kind() == reflect.Struct {
		rv := reflect.ValueOf(&result).Elem()
		for key, val := range m {
			for i := 0; i < resultType.NumField(); i++ {
				field := resultType.Field(i)
				if field.PkgPath != "" {
					continue
				}
				fname := field.Name
				if tag := field.Tag.Get("json"); tag != "" {
					fname = tag
					if idx := len(fname); idx > 0 {
						for j, ch := range fname {
							if ch == ',' {
								fname = fname[:j]
								break
							}
						}
					}
				}
				if fname == key && rv.Field(i).CanSet() {
					converted, convErr := convertValue(val, field.Type)
					if convErr != nil {
						return result, fmt.Errorf("field %q: %w", key, convErr)
					}
					rv.Field(i).Set(converted)
					break
				}
			}
		}
		return result, nil
	}
	return result, fmt.Errorf("unsupported type for conversion: %v", resultType.Kind())
}

func convertToMap(v any) (map[string]any, error) {
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Map {
		result := make(map[string]any)
		for _, key := range rv.MapKeys() {
			result[key.String()] = rv.MapIndex(key).Interface()
		}
		return result, nil
	}
	if rv.Kind() == reflect.Struct {
		result := make(map[string]any)
		t := rv.Type()
		for i := 0; i < t.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := field.Name
			if tag := field.Tag.Get("json"); tag != "" {
				name = tag
				if idx := len(name); idx > 0 {
					for j, ch := range name {
						if ch == ',' {
							name = name[:j]
							break
						}
					}
				}
			}
			result[name] = rv.Field(i).Interface()
		}
		return result, nil
	}
	return nil, fmt.Errorf("unsupported type for map conversion: %v", rv.Kind())
}

func (f *functionTool[TArgs, TResults]) Declaration() *content.ToolDeclaration {
	return &content.ToolDeclaration{
		Name:          f.Name(),
		Description:   f.Description(),
		Parameters:    f.schema,
		IsLongRunning: f.cfg.IsLongRunning,
	}
}
