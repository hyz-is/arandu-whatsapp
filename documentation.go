package whatsapp

import (
	"errors"
	"reflect"

	"github.com/arandu-io/framework/data"
	"github.com/arandu-io/framework/security"

	documentation "github.com/hyz-is/arandu-whatsapp/app/Http/Documentation"
)

// Documentation is the narrow Arandu Swagger registry contract required to
// publish the WhatsApp HTTP contract.
type Documentation = documentation.Registry

// NewWithDocumentation builds the WhatsApp module and registers its reusable
// OpenAPI components on the supplied Arandu Swagger instance. The host
// application still owns and explicitly registers the Swagger module.
func NewWithDocumentation(
	cfg Config,
	db *data.DB,
	sessions *security.SessionStore,
	docs Documentation,
) (*Module, error) {
	if isNilDocumentation(docs) {
		return nil, errors.New("whatsapp: NewWithDocumentation needs a documentation registry")
	}
	module, err := newModule(cfg, db, sessions, docs)
	if err != nil {
		return nil, err
	}
	if err := documentation.RegisterComponents(docs); err != nil {
		return nil, err
	}
	return module, nil
}

func isNilDocumentation(docs Documentation) bool {
	if docs == nil {
		return true
	}
	value := reflect.ValueOf(docs)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
