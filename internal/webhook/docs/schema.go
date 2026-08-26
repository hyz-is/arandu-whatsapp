package docs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/arandu-io/hesape/jsonschema"
)

// validateStaticEventExample validates closed contracts field by field.
// Dynamic event data validates only its declared root kind: hesape/jsonschema
// v0.12 closes objects built with Object and does not expose an open-object
// builder, while those events legitimately carry extra fields.
func validateStaticEventExample(event EventDoc) error {
	data, present := event.Example["data"]
	if !present {
		return fmt.Errorf("event %s example has no data", event.Name)
	}
	if event.DynamicFields {
		root, err := dynamicEventDataRoot(event)
		if err != nil {
			return err
		}
		if err := jsonschema.Validate(root, data); err != nil {
			return fmt.Errorf("event %s dynamic example data has an invalid root: %w", event.Name, err)
		}
		return nil
	}

	document, err := eventDataDocument(event, data)
	if err != nil {
		return err
	}
	if err := jsonschema.Validate(document.Root, data); err != nil {
		return fmt.Errorf("event %s example data does not match its schema: %w", event.Name, err)
	}
	return nil
}

func dynamicEventDataRoot(event EventDoc) (jsonschema.Type, error) {
	switch event.DataType {
	case "object":
		return jsonschema.Union("object"), nil
	case "array":
		return jsonschema.Array(), nil
	default:
		return nil, fmt.Errorf("event %s has unsupported dynamic data type %q", event.Name, event.DataType)
	}
}

func eventDataDocument(event EventDoc, example any) (jsonschema.Document, error) {
	properties, err := eventDataProperties(event)
	if err != nil {
		return jsonschema.Document{}, err
	}

	var root jsonschema.Type
	switch event.DataType {
	case "object":
		root = jsonschema.Object(properties...)
	case "array":
		root = jsonschema.Array().Items(jsonschema.Object(properties...))
	default:
		return jsonschema.Document{}, fmt.Errorf("event %s has unsupported data type %q", event.Name, event.DataType)
	}

	return jsonschema.Document{
		ID:       "urn:arandu-whatsapp:webhook-data:" + event.Name,
		Root:     root,
		Examples: []any{example},
	}, nil
}

func eventDataProperties(event EventDoc) ([]jsonschema.Property, error) {
	properties := make([]jsonschema.Property, 0, len(event.Fields))
	seen := make(map[string]struct{}, len(event.Fields))
	for _, field := range event.Fields {
		if _, duplicate := seen[field.Name]; duplicate {
			return nil, fmt.Errorf("event %s has duplicate field %s", event.Name, field.Name)
		}
		seen[field.Name] = struct{}{}

		// Dotted names describe members of an opaque nested DTO in the Markdown
		// renderer; they are not literal top-level JSON properties.
		if strings.Contains(field.Name, ".") {
			continue
		}
		fieldType, err := eventFieldType(field)
		if err != nil {
			return nil, fmt.Errorf("event %s field %s: %w", event.Name, field.Name, err)
		}
		properties = append(properties, jsonschema.Prop(field.Name, fieldType))
	}
	return properties, nil
}

func eventFieldType(field Field) (jsonschema.Type, error) {
	typeName := strings.TrimSpace(strings.TrimSuffix(field.Type, " | null"))
	switch typeName {
	case "string":
		typeSchema := jsonschema.String().Description(field.Description)
		if len(field.Values) > 0 {
			typeSchema.Enum(field.Values...)
		}
		if field.Nullable {
			typeSchema.Nullable()
		}
		if field.Required {
			typeSchema.Required()
		}
		return typeSchema, nil
	case "number":
		typeSchema := jsonschema.Number().Description(field.Description)
		if len(field.Values) > 0 {
			values := make([]float64, len(field.Values))
			for index, value := range field.Values {
				parsed, err := strconv.ParseFloat(value, 64)
				if err != nil {
					return nil, fmt.Errorf("numeric enum value %q is invalid", value)
				}
				values[index] = parsed
			}
			typeSchema.Enum(values...)
		}
		if field.Nullable {
			typeSchema.Nullable()
		}
		if field.Required {
			typeSchema.Required()
		}
		return typeSchema, nil
	case "boolean":
		if len(field.Values) > 0 {
			return nil, fmt.Errorf("boolean enums are not supported by hesape/jsonschema v0.12")
		}
		typeSchema := jsonschema.Boolean().Description(field.Description)
		if field.Nullable {
			typeSchema.Nullable()
		}
		if field.Required {
			typeSchema.Required()
		}
		return typeSchema, nil
	case "object":
		return decorateUnion(jsonschema.Union("object"), field), nil
	case "GroupParticipantWebhookData[]":
		return decorateArray(jsonschema.Array(), field), nil
	case "GroupPartialWebhookData":
		// Named DTOs are opaque nested objects here. Their fields remain domain
		// metadata for the Markdown renderer and are not duplicated in a second
		// local schema representation.
		return decorateUnion(jsonschema.Union("object"), field), nil
	default:
		return nil, fmt.Errorf("unsupported documented type %q", field.Type)
	}
}

func decorateUnion(typeSchema *jsonschema.UnionType, field Field) jsonschema.Type {
	typeSchema.Description(field.Description)
	if field.Nullable {
		typeSchema.Nullable()
	}
	if field.Required {
		typeSchema.Required()
	}
	return typeSchema
}

func decorateArray(typeSchema *jsonschema.ArrayType, field Field) jsonschema.Type {
	typeSchema.Description(field.Description)
	if field.Nullable {
		typeSchema.Nullable()
	}
	if field.Required {
		typeSchema.Required()
	}
	return typeSchema
}
