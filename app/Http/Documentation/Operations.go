package apidocs

import (
	"github.com/arandu-io/hesape/jsonschema"
	swagger "github.com/hyz-is/arandu-swagger"
)

type querySchema uint8

const (
	queryString querySchema = iota
	queryInstanceName
	queryLimit
	queryBoolean
)

type queryDefinition struct {
	name        string
	description string
	schema      querySchema
}

type bodyDefinition struct {
	contentType     string
	schemaComponent string
	required        bool
}

type mediaDefinition struct {
	contentType     string
	schemaComponent string
}

type operationResponseDefinition struct {
	status      int
	component   string
	description string
	media       []mediaDefinition
}

type operationDefinition struct {
	summary             string
	description         string
	tag                 string
	parameterComponents []string
	queries             []queryDefinition
	body                *bodyDefinition
	responses           []operationResponseDefinition
}

// Route applies the WhatsApp contract associated with a named Arandu route to
// its Swagger operation builder.
func Route(name string, builder *swagger.OperationBuilder) {
	definition, exists := operationDefinitions[name]
	if !exists || builder == nil {
		return
	}

	builder.Summary(definition.summary)
	if definition.description != "" {
		builder.Description(definition.description)
	}
	if definition.tag != "" {
		builder.Tags(definition.tag)
	}
	for _, component := range definition.parameterComponents {
		builder.ParameterRef(component)
	}
	for _, query := range definition.queries {
		options := make([]swagger.ParameterOption, 0, 1)
		if query.description != "" {
			options = append(options, swagger.Description(query.description))
		}
		builder.QueryParameter(query.name, querySchemaFor(query.schema), options...)
	}
	if definition.body != nil {
		media := swagger.MediaRef(definition.body.contentType, definition.body.schemaComponent)
		if definition.body.required {
			media.Required()
		}
		builder.RequestBody(media)
	}
	for _, response := range definition.responses {
		if response.component != "" {
			builder.ResponseRef(response.status, response.component)
			continue
		}
		media := make([]*swagger.Media, 0, len(response.media))
		for _, representation := range response.media {
			media = append(media, swagger.MediaRef(representation.contentType, representation.schemaComponent))
		}
		builder.Response(response.status, response.description, media...)
	}
	builder.Security(securitySchemeName)
}

func querySchemaFor(kind querySchema) jsonschema.Type {
	switch kind {
	case queryInstanceName:
		return jsonschema.String().Max(255)
	case queryLimit:
		return jsonschema.Integer().Min(1).Max(200).Default(200)
	case queryBoolean:
		return jsonschema.Boolean().Default(false)
	default:
		return jsonschema.String()
	}
}
