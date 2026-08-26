// Package apidocs declares the native Arandu Swagger contract contributed by
// the WhatsApp module.
package apidocs

import (
	"fmt"

	"github.com/arandu-io/framework/security"
	swagger "github.com/hyz-is/arandu-swagger"
)

const securitySchemeName = "aranduSession"

// Registry is the component and route-documentation contract required by the
// WhatsApp module. A Swagger module satisfies it without exposing lifecycle
// ownership to this package.
type Registry interface {
	swagger.Documenter
	SchemaComponent(name string, schema swagger.Schema) error
	Parameter(name string, parameter swagger.Parameter) error
	Response(name string, response swagger.Response) error
	SecurityScheme(name string, scheme swagger.SecurityScheme) error
}

type parameterDefinition struct {
	component string
	name      string
	location  string
	required  bool
	schema    string
}

type responseDefinition struct {
	component       string
	description     string
	contentType     string
	schemaComponent string
}

// RegisterComponents registers the reusable WhatsApp schemas, parameters,
// responses and Arandu session security scheme on one documentation registry.
func RegisterComponents(registry Registry) error {
	for _, definition := range schemaDefinitions {
		schema, err := swagger.SchemaFromJSON([]byte(definition.document))
		if err != nil {
			return fmt.Errorf("whatsapp documentation: parse schema %q: %w", definition.name, err)
		}
		if err := registry.SchemaComponent(definition.name, schema); err != nil {
			return fmt.Errorf("whatsapp documentation: register schema %q: %w", definition.name, err)
		}
	}

	for _, definition := range parameterDefinitions {
		schema, err := swagger.SchemaFromJSON([]byte(definition.schema))
		if err != nil {
			return fmt.Errorf("whatsapp documentation: parse parameter %q: %w", definition.component, err)
		}
		parameter := swagger.Parameter{
			Name:     definition.name,
			In:       swagger.ParameterLocation(definition.location),
			Required: definition.required,
			Schema:   &schema,
		}
		if err := registry.Parameter(definition.component, parameter); err != nil {
			return fmt.Errorf("whatsapp documentation: register parameter %q: %w", definition.component, err)
		}
	}

	for _, definition := range responseDefinitions {
		response := swagger.Response{Description: definition.description}
		if definition.schemaComponent != "" {
			schema := swagger.SchemaRef(definition.schemaComponent)
			response.Content = swagger.Content{
				definition.contentType: {Schema: &schema},
			}
		}
		if err := registry.Response(definition.component, response); err != nil {
			return fmt.Errorf("whatsapp documentation: register response %q: %w", definition.component, err)
		}
	}

	if err := registry.SecurityScheme(
		securitySchemeName,
		swagger.APIKey(security.SessionCookieName, swagger.APIKeyInCookie),
	); err != nil {
		return fmt.Errorf("whatsapp documentation: register Arandu session security scheme: %w", err)
	}
	return nil
}
